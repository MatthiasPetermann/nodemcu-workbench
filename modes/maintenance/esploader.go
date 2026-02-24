package maintenance

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go.bug.st/serial"
)

const (
	espFlashWriteSize = 0x400
	espChecksumMagic  = 0xEF
)

type espClient struct {
	port serial.Port
}

type deviceInfo struct {
	Chip       string
	ChipID     uint32
	Mac        string
	MagicValue uint32
}

func openESPClient(portName string, baud int) (*espClient, error) {
	p, err := serial.Open(portName, &serial.Mode{BaudRate: baud})
	if err != nil {
		return nil, err
	}
	_ = p.SetReadTimeout(200 * time.Millisecond)
	return &espClient{port: p}, nil
}

func (c *espClient) Close() error { return c.port.Close() }

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
	if _, _, err := c.command(0x08, payload, 0, 500*time.Millisecond); err != nil {
		return err
	}
	for i := 0; i < 7; i++ {
		if _, _, err := c.readPacket(500 * time.Millisecond); err != nil {
			break
		}
	}
	return nil
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
	const eraseSize = 4 * 1024 * 1024
	_, err := c.flashBegin(eraseSize, 0)
	if err != nil {
		return err
	}
	return c.flashEnd(false)
}

func (c *espClient) flashFirmware(path string, offset uint32) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	blocks, err := c.flashBegin(len(data), offset)
	if err != nil {
		return err
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
			return err
		}
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
