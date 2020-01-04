package main

import (
	"fmt"
	"net"
	"strconv"
	"encoding/json"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const (
	RemoteReceivePort int = 10181
	RemoteSendPort 	  int = 10182
)

func broadcast(message []byte) error{
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

func receive() {
	socket, err := net.ListenUDP("udp",  &net.UDPAddr{IP: net.IPv4zero, Port: RemoteReceivePort})
	if err != nil {
		fmt.Println(err)
		return
	}

	data := make([]byte, 1024)
	fmt.Printf("Local: <%s> \n", socket.LocalAddr().String())

	for {
		n, addr, err := socket.ReadFromUDP(data)
		if err != nil {
			fmt.Println("Error occurred while reading: %s", err)
		}

		fmt.Printf("<%s> %s\n", addr, data[:n])

		go processing(data[:n])
	}
}

type processor func(map[string]interface{})

func processing(message []byte) {
	var f interface{}

	err := json.Unmarshal(message, &f)

	if err != nil {
		fmt.Println(err)
	}

	msg := f.(map[string]interface{})
	fmt.Printf("%s\n", msg)
}

func newMQTTClientOptions(uri, port, username, password string) *MQTT.ClientOptions {
	opts := MQTT.NewClientOptions()

	if port == "" {
		port = "1883"
	}

	broker := "tcp://" + uri + ":" + port
	opts.AddBroker(broker)
	opts.SetClientID("SmartControl CLI")

	if username != "" {
		opts.SetUsername(username)
	}

	if password != "" {
		opts.SetPassword(password)
	}

	return opts
}

func publish(topic string, message []byte, uri, port, username, password string) error {
	opts := newMQTTClientOptions(uri, port, username, password)
	client := MQTT.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	defer client.Disconnect(250)
	
	t := client.Publish(topic, byte(0), false, message)
	t.Wait()

	return nil
}

func subscribe(topic, uri, port, username, password string) {
	opts := newMQTTClientOptions(uri, port, username, password)
	choke := make(chan MQTT.Message)

	opts.SetDefaultPublishHandler(func(client MQTT.Client, msg MQTT.Message) {
		choke <- msg
	})

	client := MQTT.NewClient(opts)
	defer client.Disconnect(250)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		return
	}

	if token := client.Subscribe(topic, byte(1), nil); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		return
	}

	for {
		msg := <- choke
		go processing(msg.Payload())
	}
}

// Discover send an UDP request and actively listen on device feedback 
func Discover() {
	go broadcast([]byte(`{"cmd": "device report"}`))

	receive()
}

// AdoptDevice initialize MQTT settings to device via UDP
func AdoptDevice(mac, uri, port, username, password string) error {
	payload := map[string]interface{}{
		"mac": mac,
		"setting": map[string]interface{}{
			"mqtt_uri": uri,
			"mqtt_port": port,
			"mqtt_user": username,
			"mqtt_password": password,
		},
	}

	msg, err := json.Marshal(payload)
	if err != nil {
		fmt.Println(err)
		return err
	}

	broadcast(msg)

	return nil
}

// ActivateDevice activates the device by given code
func ActivateDevice(mac, code, uri, port, username, password string) error {
	payload := map[string]interface{}{
		"mac": mac,
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
		return err
	}

	return nil
}

// DevicePower subscribe MQTT topic to get device sensor information (power/uptime)
func DevicePower(mac, uri, port, username, password string) {
	topic := "device/ztc1/" + mac + "/sensor"

	subscribe(topic, uri, port, username, password)
}

// DeviceState subscribe MQTT topic to get device state information (plug on/off state)
func DeviceState(mac, uri, port, username, password string) {
	topic := "device/ztc1/" + mac + "/state"

	subscribe(topic, uri, port, username, password)
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
		return err
	}

	return nil
}

// UpgradeDevice upgrades device via an OTA uri
func UpgradeDevice(mac, uri, port, username, password, otaUri string) error {
	topic := "device/ztc1/" + mac + "/set"

	payload := map[string]interface{}{
		"mac": mac,
		"setting": map[string]interface{}{
			"ota": otaUri,
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

	// after upgrade it would use UDP to receive progress
	go receive()

	return nil
}

func main() {
	Discover()
}