package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type DiscoveryDevice struct {
	Identifiers  string `json:"identifiers"`
	Name         string `json:"name"`
	SwVersion    string `json:"sw_version"`
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
}

type Discovery struct {
	DeviceClass string          `json:"device_class"`
	StateTopic  string          `json:"state_topic"`
	Name        string          `json:"name"`
	UniqueID    string          `json:"unique_id"`
	Device      DiscoveryDevice `json:"device"`
}

type Device struct {
	name string
	mac  string
}

var devices = []Device{
	Device{
		name: "Pixel",
		mac:  "40:4E:36:xx:xx:xx",
	},
	Device{
		name: "MacBook",
		mac:  "6C:96:CF:xx:xx:xx",
	},
}

const (
	MQTT_ADDRESS = "tcp://localhost:1883"
	CLIENT_ID    = "wifi-presence"
)

func findDevice(line string) (Device, bool) {
	macRegex := regexp.MustCompile("([[:xdigit:]]{2}[:-]){5}([[:xdigit:]]{2})")

	mac := macRegex.FindString(line)
	mac = strings.ToUpper(strings.ReplaceAll(mac, "-", ":"))

	for _, device := range devices {
		validMac := mac == device.mac
		if validMac {
			return device, true
		}
	}
	return Device{
		name: fmt.Sprintf("unknown device (%s)", mac),
		mac:  mac,
	}, false
}

func main() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(MQTT_ADDRESS)
	opts.SetClientID(CLIENT_ID)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	addr, err := net.ResolveUDPAddr("udp4", ":9002")
	if err != nil {
		panic(err)
	}

	sock, err := net.ListenUDP("udp4", addr)
	if err != nil {
		panic(err)
	}
	defer sock.Close()
	log.Println("Listening at ", addr)

	buf := make([]byte, 1024)

	for _, device := range devices {
		topic := strings.Replace(device.mac, ":", "", -1)
		id := topic + "_" + device.name + "_wifipresence"

		discovery := Discovery{
			Name:        device.name,
			DeviceClass: "presence",
			StateTopic:  "device/wifi/" + device.name + "/status",
			UniqueID:    id,
			Device: DiscoveryDevice{
				Identifiers:  id,
				Name:         device.name,
				SwVersion:    "wifipresence 1.0",
				Model:        "Wifi Device",
				Manufacturer: "Jakub Czekański",
			},
		}
		j, _ := json.Marshal(discovery)
		client.Publish("homeassistant/binary_sensor/"+topic+"/config", 0, true, string(j))
	}

	for {
		n, _, err := sock.ReadFromUDP(buf)

		if err != nil {
			log.Println("Error: ", err)
		}

		line := string(buf[:n])

		disconnected := strings.Contains(line, " disconnected")
		connected := strings.Contains(line, " connected")

		if !connected && !disconnected {
			log.Printf("[?] %s\n", line)
			continue
		}

		device, knownDevice := findDevice(line)

		if connected {
			log.Printf("[+] %s connected\n", device.name)
			if knownDevice {
				client.Publish("device/wifi/"+device.name+"/status", 0, true, "ON")
			}
		} else if disconnected {
			log.Printf("[-] %s disconnected\n", device.name)
			if knownDevice {
				client.Publish("device/wifi/"+device.name+"/status", 0, true, "OFF")
			}
		}
	}
}
