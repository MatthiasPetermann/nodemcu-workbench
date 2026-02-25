package maintenance

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.bug.st/serial"

	"nodemcu-workbench/repl"
)

const (
	espFlashWriteSize = 0x400
	espChecksumMagic  = 0xEF
)

type espClient struct {
	port  serial.Port
	owned bool
}

type deviceInfo struct {
	Chip       string
	ChipID     uint32
	Mac        string
	MagicValue uint32
}

type flashSegment struct {
	Offset uint32
	Path   string
}

type flashProgressFn func(phase string, done, total int)

func openESPClient(portName string, baud int) (*espClient, error) {
	ports := candidatePorts(portName)
	var lastErr error
	for _, pth := range ports {
		p, err := serial.Open(pth, &serial.Mode{BaudRate: baud})
		if err != nil {
			lastErr = err
			continue
		}
		_ = p.SetReadTimeout(200 * time.Millisecond)
		return &espClient{port: p, owned: true}, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("open serial failed (tried %v): %w", ports, lastErr)
	}
	return nil, fmt.Errorf("open serial failed: no candidate ports")
}

func espClientFromSession(sess *repl.Session) (*espClient, error) {
	if sess == nil {
		return nil, fmt.Errorf("not connected")
	}
	var p serial.Port
	err := sess.WithExclusivePort(func(port serial.Port) error {
		p = port
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &espClient{port: p, owned: false}, nil
}

func candidatePorts(preferred string) []string {
	seen := map[string]struct{}{}
	add := func(out *[]string, p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		*out = append(*out, p)
	}

	out := make([]string, 0, 8)
	add(&out, preferred)

	patterns := []string{"/dev/ttyUSB*", "/dev/ttyACM*", "/dev/cu.usb*", "/dev/tty.SLAB*"}
	for _, pat := range patterns {
		matches, _ := filepath.Glob(pat)
		sort.Strings(matches)
		for _, m := range matches {
			add(&out, m)
		}
	}

	return out
}

func (c *espClient) Close() error {
	if !c.owned {
		return nil
	}
	return c.port.Close()
}

func (c *espClient) hardReset() {
	// EN low/high pulse via RTS, matching common ESP reset wiring.
	_ = c.port.SetRTS(true)
	time.Sleep(120 * time.Millisecond)
	_ = c.port.SetRTS(false)
	time.Sleep(120 * time.Millisecond)
	_ = c.port.SetDTR(false)
}

func (c *espClient) enterBootloader() {
	_ = c.port.SetDTR(false)
	_ = c.port.SetRTS(true)
	time.Sleep(100 * time.Millisecond)
	_ = c.port.SetDTR(true)
	_ = c.port.SetRTS(false)
	time.Sleep(50 * time.Millisecond)
	_ = c.port.SetDTR(false)
	time.Sleep(50 * time.Millisecond)
	_ = c.port.ResetInputBuffer()
	_ = c.port.ResetOutputBuffer()
}

func (c *espClient) sync() error {
	payload := append([]byte{0x07, 0x07, 0x12, 0x20}, bytes.Repeat([]byte{0x55}, 32)...)

	var lastErr error
	for attempt := 0; attempt < 7; attempt++ {
		_ = c.port.ResetInputBuffer()
		_ = c.port.ResetOutputBuffer()

		if _, _, err := c.command(0x08, payload, 0, 3*time.Second); err == nil {
			// ROM often sends additional sync responses; drain a few opportunistically.
			for i := 0; i < 7; i++ {
				if _, _, err := c.readPacket(150 * time.Millisecond); err != nil {
					break
				}
			}
			return nil
		} else {
			lastErr = err
		}

		time.Sleep(100 * time.Millisecond)
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("sync failed")
}

func (c *espClient) identify() (deviceInfo, error) {
	magic, err := c.readReg(0x40001000)
	if err != nil {
		return deviceInfo{}, err
	}
	id0, err := c.readReg(0x3FF00050)
	if err != nil {
		return deviceInfo{}, err
	}
	id1, err := c.readReg(0x3FF00054)
	if err != nil {
		return deviceInfo{}, err
	}

	chipID := (id0 >> 24) | ((id1 & 0xFFFFFF) << 8)
	mac0 := id1
	mac1 := id0
	mac := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		byte(mac1>>8), byte(mac1), byte(mac0>>24), byte(mac0>>16), byte(mac0>>8), byte(mac0),
	)

	chip := "Unknown"
	switch magic {
	case 0xFFF0C101:
		chip = "ESP8266"
	case 0x00F01D83:
		chip = "ESP32"
	case 0x000007C6:
		chip = "ESP32-S2"
	}

	return deviceInfo{Chip: chip, ChipID: chipID, Mac: mac, MagicValue: magic}, nil
}

func (c *espClient) eraseFlash() error {
	eraseSize := 4 * 1024 * 1024

	// Try ROM upfront-erase first. Some ROMs reject this combination with
	// status/reason errors (for example reason=6), then fallback to explicitly
	// writing 0xFF blocks which also results in an erased flash region.
	if _, err := c.flashBegin(eraseSize, 0); err == nil {
		return c.flashEnd(false)
	} else if !isROMReason(err, 6) {
		return err
	}

	blocks, err := c.flashBegin(eraseSize, 0)
	if err != nil {
		return err
	}

	blank := bytes.Repeat([]byte{0xFF}, espFlashWriteSize)
	for seq := 0; seq < blocks; seq++ {
		if err := c.flashData(blank, seq); err != nil {
			return err
		}
	}

	return c.flashEnd(false)
}

func (c *espClient) flashFirmware(path string, offset uint32) error {
	return c.flashImages([]flashSegment{{Offset: offset, Path: path}}, nil)
}

func (c *espClient) flashImages(segments []flashSegment, progress flashProgressFn) error {
	if len(segments) == 0 {
		return fmt.Errorf("no firmware segments provided")
	}
	if progress != nil {
		progress("prepare", 0, 1)
	}

	totalAll := 0
	images := make([][]byte, len(segments))
	for idx, seg := range segments {
		data, err := readFirmwareData(seg.Path)
		if err != nil {
			return fmt.Errorf("segment %d (%s): %w", idx, seg.Path, err)
		}
		images[idx] = data
		totalAll += len(data)
	}

	doneAll := 0
	for idx, seg := range segments {
		data := images[idx]
		if progress != nil {
			progress(fmt.Sprintf("flash 0x%08x", seg.Offset), doneAll, totalAll)
		}

		blocks, err := c.flashBegin(len(data), seg.Offset)
		if err != nil {
			return fmt.Errorf("segment %d (%s @0x%08x): %w", idx, seg.Path, seg.Offset, err)
		}

		for seq := 0; seq < blocks; seq++ {
			start := seq * espFlashWriteSize
			end := start + espFlashWriteSize
			if end > len(data) {
				end = len(data)
			}
			blk := make([]byte, espFlashWriteSize)
			copy(blk, data[start:end])
			if err := c.flashData(blk, seq); err != nil {
				return fmt.Errorf("segment %d (%s @0x%08x block=%d): %w", idx, seg.Path, seg.Offset, seq, err)
			}
			doneAll += end - start
			if progress != nil {
				progress(fmt.Sprintf("flash 0x%08x", seg.Offset), doneAll, totalAll)
			}
		}
	}

	if progress != nil {
		progress("finish", totalAll, totalAll)
	}
	return c.flashEnd(true)
}

func (c *espClient) flashBegin(size int, offset uint32) (int, error) {
	blocks := (size + espFlashWriteSize - 1) / espFlashWriteSize
	payload := make([]byte, 16)
	binary.LittleEndian.PutUint32(payload[0:4], uint32(size))
	binary.LittleEndian.PutUint32(payload[4:8], uint32(blocks))
	binary.LittleEndian.PutUint32(payload[8:12], espFlashWriteSize)
	binary.LittleEndian.PutUint32(payload[12:16], offset)
	_, _, err := c.command(0x02, payload, 0, 30*time.Second)
	return blocks, err
}

func (c *espClient) flashData(data []byte, seq int) error {
	payload := make([]byte, 16+len(data))
	binary.LittleEndian.PutUint32(payload[0:4], uint32(len(data)))
	binary.LittleEndian.PutUint32(payload[4:8], uint32(seq))
	copy(payload[16:], data)
	_, _, err := c.command(0x03, payload, checksum(data), 5*time.Second)
	return err
}

func (c *espClient) flashEnd(reboot bool) error {
	payload := make([]byte, 4)
	if !reboot {
		binary.LittleEndian.PutUint32(payload, 1)
	}
	_, _, err := c.command(0x04, payload, 0, 3*time.Second)
	return err
}

func (c *espClient) readReg(addr uint32) (uint32, error) {
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, addr)
	val, _, err := c.command(0x0A, payload, 0, 2*time.Second)
	return val, err
}

func (c *espClient) command(op byte, data []byte, chk uint32, timeout time.Duration) (uint32, []byte, error) {
	pkt := make([]byte, 8+len(data))
	pkt[0] = 0x00
	pkt[1] = op
	binary.LittleEndian.PutUint16(pkt[2:4], uint16(len(data)))
	binary.LittleEndian.PutUint32(pkt[4:8], chk)
	copy(pkt[8:], data)

	if err := c.writePacket(pkt); err != nil {
		return 0, nil, err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		val, payload, err := c.readPacket(time.Until(deadline))
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "timeout") {
				continue
			}
			return 0, nil, err
		}
		if len(payload) < 8 {
			continue
		}
		if payload[1] != op {
			continue
		}
		resp := payload[8:]
		if len(resp) >= 2 && resp[len(resp)-2] != 0 {
			return 0, nil, fmt.Errorf("rom error: status=%d reason=%d", resp[len(resp)-2], resp[len(resp)-1])
		}
		return val, resp, nil
	}
	return 0, nil, fmt.Errorf("timeout waiting for response (op=0x%02x)", op)
}

func (c *espClient) writePacket(raw []byte) error {
	enc := []byte{0xC0}
	for _, b := range raw {
		switch b {
		case 0xC0:
			enc = append(enc, 0xDB, 0xDC)
		case 0xDB:
			enc = append(enc, 0xDB, 0xDD)
		default:
			enc = append(enc, b)
		}
	}
	enc = append(enc, 0xC0)
	_, err := c.port.Write(enc)
	return err
}

func (c *espClient) readPacket(timeout time.Duration) (uint32, []byte, error) {
	if timeout <= 0 {
		timeout = 50 * time.Millisecond
	}
	_ = c.port.SetReadTimeout(timeout)

	buf := make([]byte, 1)
	payload := make([]byte, 0, 128)
	started := false

	for {
		n, err := c.port.Read(buf)
		if err != nil {
			return 0, nil, err
		}
		if n == 0 {
			return 0, nil, io.EOF
		}
		b := buf[0]

		if !started {
			if b == 0xC0 {
				started = true
				payload = payload[:0]
			}
			continue
		}

		if b == 0xC0 {
			if len(payload) < 8 {
				started = false
				continue
			}
			if payload[0] != 0x01 {
				started = false
				continue
			}
			val := binary.LittleEndian.Uint32(payload[4:8])
			return val, payload, nil
		}

		if b == 0xDB {
			n, err = c.port.Read(buf)
			if err != nil {
				return 0, nil, err
			}
			if n == 0 {
				return 0, nil, io.EOF
			}
			switch buf[0] {
			case 0xDC:
				b = 0xC0
			case 0xDD:
				b = 0xDB
			default:
				return 0, nil, fmt.Errorf("invalid slip escape")
			}
		}

		payload = append(payload, b)
	}
}

func checksum(data []byte) uint32 {
	var c byte = espChecksumMagic
	for _, b := range data {
		c ^= b
	}
	return uint32(c)
}

func isROMReason(err error, reason int) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), fmt.Sprintf("reason=%d", reason))
}
