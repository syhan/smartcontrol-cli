package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const (
	// RemoteReceivePort remote device port to receive data
	RemoteReceivePort int = 10181
	// RemoteSendPort remote device port to send data
	RemoteSendPort int = 10182
	// interval of the progress bar dots
	interval time.Duration = 1000 * time.Millisecond
)

type processor func(map[string]interface{})

// process handles the message
func process(message []byte, proc processor) {
	var f interface{}

	err := json.Unmarshal(message, &f)

	if err != nil {
		fmt.Println(err)
	}

	msg := f.(map[string]interface{})
	proc(msg)
}

type send func(message []byte) error
type recv func()

// broadcast to the local network via UDP
func broadcast(message []byte) error {
	addr := &net.UDPAddr{IP: net.IPv4bcast, Port: RemoteSendPort}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		fmt.Println(err)
		return err
	}

	_, err = conn.WriteToUDP(message, addr)

	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

// receive listens on specific port
func receive(recvCh chan []byte, stopCh chan struct{}) {
	socket, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: RemoteReceivePort})
	if err != nil {
		fmt.Println(err)
		return
	}

	data := make([]byte, 1024)

	go func() {
		for {
			n, _, err := socket.ReadFromUDP(data)
			if err != nil {
				fmt.Println(err)
				continue
			}

			recvCh <- data[:n]

		}
	}()

	<-stopCh
	socket.Close()
}

// newMQTTClientOptions initializes a client options, only uri is mandantory
func newMQTTClient(uri, port, username, password string) MQTT.Client {
	opts := MQTT.NewClientOptions()

	broker := "tcp://" + uri + ":" + port
	opts.AddBroker(broker)
	opts.SetClientID("SmartControl-CLI")

	if username != "" {
		opts.SetUsername(username)
	}

	if password != "" {
		opts.SetPassword(password)
	}

	return MQTT.NewClient(opts)
}

// publish sends message on a specific topic
func publish(topic string, message []byte, uri, port, username, password string) error {
	client := newMQTTClient(uri, port, username, password)
	defer client.Disconnect(250)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	t := client.Publish(topic, byte(0), false, message)
	t.Wait()

	return nil
}

// subscribe receives message on a specific topic
func subscribe(topic, uri, port, username, password string, recvCh chan []byte, stopCh chan struct{}) {
	client := newMQTTClient(uri, port, username, password)
	defer client.Disconnect(250)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		return
	}

	handler := func(client MQTT.Client, msg MQTT.Message) {
		recvCh <- msg.Payload()
	}

	if token := client.Subscribe(topic, byte(1), handler); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		return
	}

	<-stopCh
}

// Discover send an UDP request and actively listen on device feedback
func Discover() {
	fmt.Print("Broadcast to the local area network, ")
	go broadcast([]byte(`{"cmd": "device report"}`))

	tick := time.Tick(interval)
	recvCh := make(chan []byte)
	stopCh := make(chan struct{})
	var signal struct{}

	fmt.Print("wait for device to report.\n")
	go receive(recvCh, stopCh)

	proc := func(r map[string]interface{}) {
		stopCh <- signal
		name, _ := r["name"]
		mac, _ := r["mac"]
		deviceType, _ := r["type_name"]
		ip, _ := r["ip"]

		fmt.Printf("Device found! Type: %s, Name: %s, Mac: %s, IP: %s\n", deviceType, name, mac, ip)
	}

	for {
		select {
		case <-tick:
			fmt.Print(".") // progress bar, maximumly 60 dots would be printed
		case resp := <-recvCh:
			process(resp, proc)

			return
		case <-time.After(30 * time.Second):
			fmt.Println("Timeout finding device, consider UDP is not reliable, you may try again later")
		}
	}
}

// AdoptDevice initialize MQTT settings to device via UDP
func AdoptDevice(mac, uri, port, username, password string) error {
	payload := map[string]interface{}{
		"mac": mac,
		"setting": map[string]interface{}{
			"mqtt_uri":      uri,
			"mqtt_port":     port,
			"mqtt_user":     username,
			"mqtt_password": password,
		},
	}

	msg, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return err
	}

	fmt.Println("Adopt by sending MQTT server information to device")
	broadcast(msg)

	return nil
}

// ActivateDevice activates the device by given code
func ActivateDevice(mac, code, uri, port, username, password string) error {
	payload := map[string]interface{}{
		"mac":  mac,
		"lock": code,
	}

	msg, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return err
	}

	topic := "device/ztc1/" + mac + "/set"
	err = publish(topic, msg, uri, port, username, password)

	if err != nil {
		fmt.Println(err)

		// consider using UDP as a fallback
		fmt.Printf("MQTT server is not available, use UDP broadcast")
		err = broadcast(msg)
		return err
	}

	return nil
}

// DevicePower subscribe MQTT topic to get device sensor information (power/uptime)
func DevicePower(mac, uri, port, username, password string) {
	topic := "device/ztc1/" + mac + "/sensor"
	recvCh := make(chan []byte)

	proc := func(r map[string]interface{}) {
		power, _ := r["power"]
		uptime, _ := r["total_time"]

		fmt.Printf("Power: %sW, Uptime: %d seconds\n", power, int(uptime.(float64)))
	}

	go subscribe(topic, uri, port, username, password, recvCh, nil)

	for {
		msg := <-recvCh
		process(msg, proc)
	}
}

// DeviceState subscribe MQTT topic to get device state information (plug on/off state)
func DeviceState(mac, uri, port, username, password string) {
	topic := "device/ztc1/" + mac + "/state"
	recvCh := make(chan []byte)
	stopCh := make(chan struct{})

	proc := func(r map[string]interface{}) {
		var signal struct{}
		stopCh <- signal

		index := 0
		plugState := ""

		for index <= 5 {
			plug, _ := r["plug_"+strconv.Itoa(index)]
			state := plug.(map[string]interface{})
			on, _ := state["on"]

			plugState += fmt.Sprintf("Plug %d: %t\n", index, int(on.(float64)) == 1)

			index++
		}

		fmt.Println(plugState)
	}

	go subscribe(topic, uri, port, username, password, recvCh, stopCh)

	msg := <-recvCh
	process(msg, proc)
}

// SwitchPlug switches specific plug (0~5) on/off state
func SwitchPlug(mac, uri, port, username, password string, plugIndex int, on bool) error {
	topic := "device/ztc1/" + mac + "/set"

	onFlag := 0
	if on {
		onFlag = 1
	}

	payload := map[string]interface{}{
		"mac": mac,
		"plug_" + strconv.Itoa(plugIndex): map[string]interface{}{
			"on": onFlag,
		},
	}

	msg, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return err
	}

	err = publish(topic, msg, uri, port, username, password)
	if err != nil {
		fmt.Println(err)

		// consider using UDP as a fallback
		fmt.Printf("MQTT server is not available, use UDP broadcast")
		err = broadcast(msg)
		return err
	}

	// Wait some time and check the state
	time.Sleep(2 * time.Second)
	DeviceState(mac, uri, port, username, password)

	return nil
}

// UpgradeDevice upgrades device via an OTA uri
func UpgradeDevice(mac, uri, port, username, password, otaURI string) error {
	topic := "device/ztc1/" + mac + "/set"

	payload := map[string]interface{}{
		"mac": mac,
		"setting": map[string]interface{}{
			"ota": otaURI,
		},
	}

	msg, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return err
	}

	err = publish(topic, msg, uri, port, username, password)
	if err != nil {
		fmt.Println(err)
		return err
	}

	var signal struct{}

	// after upgrade it would use UDP to receive progress
	recvCh := make(chan []byte)
	stopCh := make(chan struct{})
	receive(recvCh, stopCh)

	proc := func(r map[string]interface{}) {
		progress, _ := r["ota_progress"]

		if progress.(float64) >= 100 {
			stopCh <- signal
		}

		fmt.Printf("Upgrade progress: %d%%\n", progress)
	}

	for {
		select {
		case resp := <-recvCh:
			process(resp, proc)
		default:
			time.Sleep(3 * time.Second)
		}
	}
}

var (
	mac    string
	device string

	monitorType string

	lock string

	uri      string
	port     string
	username string
	password string

	ota string

	plug int
	on   bool

	adopt    *flag.FlagSet
	activate *flag.FlagSet
	monitor  *flag.FlagSet
	upgrade  *flag.FlagSet
	switches *flag.FlagSet
)

func init() {
	setMQTT := func(f *flag.FlagSet) {
		f.StringVar(&uri, "uri", "0.0.0.0", "MQTT uri")
		f.StringVar(&port, "port", "1883", "MQTT port, optional")
		f.StringVar(&username, "username", "", "MQTT username, optional")
		f.StringVar(&password, "password", "", "MQTT password, optional")
	}

	adopt = flag.NewFlagSet("adopt", flag.ExitOnError)
	adopt.StringVar(&mac, "mac", "", "Device mac address")
	setMQTT(adopt)

	activate = flag.NewFlagSet("activiate", flag.ExitOnError)
	activate.StringVar(&mac, "mac", "", "Device mac address")
	activate.StringVar(&lock, "code", "", "Activate code")
	setMQTT(activate)

	monitor = flag.NewFlagSet("monitor", flag.ExitOnError)
	monitor.StringVar(&mac, "mac", "", "Device mac address")
	monitor.StringVar(&device, "device", "ztc1", "Device type")
	monitor.StringVar(&monitorType, "monitor", "state", "Monitor type, could be either power or state")
	setMQTT(monitor)

	upgrade = flag.NewFlagSet("upgrade", flag.ExitOnError)
	upgrade.StringVar(&mac, "mac", "", "Device mac address")
	upgrade.StringVar(&device, "device", "ztc1", "Device type")
	upgrade.StringVar(&ota, "ota", "", "OTA address")
	setMQTT(upgrade)

	switches = flag.NewFlagSet("switch", flag.ExitOnError)
	switches.StringVar(&mac, "mac", "", "Device mac address")
	switches.StringVar(&device, "device", "ztc1", "Device type")
	switches.IntVar(&plug, "plug", 0, "Plug index")
	switches.BoolVar(&on, "on", false, "Plug on/off")
	setMQTT(switches)
}

func usage() {
	fmt.Printf("usage: %s <command> [<args>]\n\n", os.Args[0])
	fmt.Println("  discover    ask device to report itself")
	fmt.Println("  adopt       send local MQTT server information to device")
	fmt.Println("  activate    activate device by given code, it requires the device has been adopted")
	fmt.Println("  monitor     monitor device status (power and uptime)")
	fmt.Println("  switch      switch a specific plug on/off")
	fmt.Println("  upgrade     upgrade device to a certain firmware")
}

func main() {
	if len(os.Args) == 1 {
		usage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "discover":
		Discover()
	case "adopt":
		adopt.Parse(os.Args[2:])
		AdoptDevice(mac, uri, port, username, password)
	case "activate":
		activate.Parse(os.Args[2:])
		ActivateDevice(mac, lock, uri, port, username, password)
	case "monitor":
		monitor.Parse(os.Args[2:])

		if monitorType == "power" {
			DevicePower(mac, uri, port, username, password)
		} else if monitorType == "state" {
			DeviceState(mac, uri, port, username, password)
		} else {
			fmt.Printf("'%s' is not a valid monitor type\n", monitorType)
			os.Exit(-1)
		}
	case "switch":
		switches.Parse(os.Args[2:])
		if plug < 0 || plug > 5 {
			fmt.Printf("Plug index value should between 0 and 5\n")
			os.Exit(-1)
		}
		SwitchPlug(mac, uri, port, username, password, plug, on)
	case "upgrade":
		upgrade.Parse(os.Args[2:])
		UpgradeDevice(mac, uri, port, username, password, ota)
	default:
		fmt.Printf("%s: '%s' is not a valid command\n", os.Args[0], os.Args[1])
		os.Exit(-1)
	}

}
