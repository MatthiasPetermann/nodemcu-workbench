-- init.lua

local pin = 4
gpio.mode(pin, gpio.OUTPUT)

while true do
  gpio.write(pin, gpio.HIGH)
  tmr.delay(500000)  -- 500 ms
  gpio.write(pin, gpio.LOW)
  tmr.delay(500000)  -- 500 ms
end
