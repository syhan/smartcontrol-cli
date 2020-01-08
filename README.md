# SmartControl CLI

A command line interface for [SmartControl](https://github.com/a2633063/SmartControl_Android_MQTT) inspired by its [communication protocol](https://github.com/a2633063/zTC1/wiki/%E9%80%9A%E4%BF%A1%E5%8D%8F%E8%AE%AE), currently it <b>ONLY</b> supports zTC1

## Usage

```
usage: zcontrol.exe <command> [<args>]

  discover    ask device to report itself
  adopt       send local MQTT server information to device
  activate    activate device by given code, it requires the device has been adopted
  monitor     monitor device status (power and uptime)
  switch      switch a specific plug on/off
  upgrade     upgrade device to a certain firmware
```

### Discover
Broadcast a UDP request to the whole local area network and wait for device to report  
`$ zcontrol.exe discover`  
ideally it would get a feedback like
`Device found! Type: zTC1, Name: zTC1_e10d, Mac: 11111111e10d, IP: 10.0.0.123`  

### Adopt
Adopt device by setting MQTT broker/port/username/password via UDP as MQTT is more reliable than UDP  
`$ zcontrol.exe adopt -mac 11111111e10d -uri 10.0.0.124 -port 1883 -username admin -password admin`  

### Activate
Activate device by sending activation [key](https://github.com/a2633063/SmartControl_Android_MQTT/wiki/%E6%BF%80%E6%B4%BB%E7%A0%81%E8%8E%B7%E5%8F%96), if MQTT server is not provided or not available, it would try to use UDP instead  
`$ zcontrol.exe activate -code 1234567890abcdefghijklmnopqrstuv -mac 11111111e10d -uri 10.0.0.124 -port 1883 -username admin -password admin`  

### Monitor
Monitor device status/power information
#### power
`$ zcontrol.exe monitor -mac 11111111e10d -monitor power -uri 10.0.0.124`  
ideally it would response  
```
Power: 5W, Uptime: 42 seconds
Power: 10W, Uptime: 47 seconds
...
```
#### state
`$ zcontrol.exe monitor -mac 11111111e10d -monitor state -uri 10.0.0.124`  
ideally it would response  
```
Plug 0: true
Plug 1: true
Plug 2: true
Plug 3: true
Plug 4: true
Plug 5: true
```

### Switch
Control each plug switch on/off status, if MQTT server is not provided or not available, it would try to use UDP instead  
- Switch plug 0 on  `$ zcontrol.exe switch -mac 11111111e10d -plug 0 -on -uri 10.0.0.124`  
- Switch plug 2 off `$ zcontrol.exe switch -mac 11111111e10d -plug 2`

Schedule is not yet supported

### Upgrade
Upgrade device via OTA  
`$ zcontrol.exe -mac 11111111e10d -ota http://192.168.43.119/TC1_MK3031_moc.ota.bin -uri 10.0.0.124`
