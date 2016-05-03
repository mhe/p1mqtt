// A simple program that reads data from the P1 port of smart meter and
// publishes it on an mqtt server.
// Copyright (c) 2016 Maarten Everts. See LICENSE.

package main

import (
	"flag"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/mhe/dsmr4p1"
	"github.com/tarm/serial"
	"github.com/ugorji/go/codec"
	"github.com/yosssi/gmq/mqtt"
	"github.com/yosssi/gmq/mqtt/client"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

var configfile = flag.String("config", "p1mqtt.toml", "Configuration file")
var testfile = flag.String("testfile", "", "Testfile to use instead of serial port")
var ratelimit = flag.Int("ratelimit", 0, "When using a testfile as input, rate-limit the release of P1 telegrams to once every n seconds.")

type Config struct {
	Encoding       string
	DefaultKey     string
	DefaultUnitKey string

	P1 struct {
		Device   string
		Baudrate int
	}
	Mqtt struct {
		BaseTopic string
		Host      string
		ClientID  string
		QoS       byte
	}
	Timestamp struct {
		OBIS   string
		Key    string
		Format string
	}
	Outputs []*struct {
		Topic string
		Keys  []*struct {
			Name          string
			UnitName      string
			Identifier    string
			Type          string
			Parser        string
			Delta         bool
			previousValue float64
			previousValid bool
		}
		lastTimestamp time.Time
	}
}

// Read configuration from file and set some defaults
func getConfig(filename string) Config {
	var conf Config
	if _, err := toml.DecodeFile(filename, &conf); err != nil {
		log.Fatal(err)
	}

	// Set the defaults
	for _, output := range conf.Outputs {
		output.Topic = conf.Mqtt.BaseTopic + output.Topic
		for _, key := range output.Keys {
			if key.Name == "" {
				key.Name = conf.DefaultKey
			}
			if key.UnitName == "" {
				key.UnitName = conf.DefaultUnitKey
			}
		}
	}
	return conf
}

func main() {
	fmt.Println("p1mqtt")

	flag.Parse()

	var err error

	absPathConfigFile, err := filepath.Abs(*configfile)
	log.Printf("Configuration file: %s\n", absPathConfigFile)

	conf := getConfig(*configfile)

	// Create an MQTT Client.
	cli := client.New(&client.Options{
		ErrorHandler: func(err error) {
			fmt.Println(err)
		},
	})

	// Terminate the Client, eventually.
	defer cli.Terminate()

	// Connect to the MQTT Server.
	err = cli.Connect(&client.ConnectOptions{
		Network:  "tcp",
		Address:  conf.Mqtt.Host,
		ClientID: []byte(conf.Mqtt.ClientID),
	})
	if err != nil {
		panic(err)
	}

	// Determine whether to use a test file or a real serial device.
	var input io.Reader
	if *testfile == "" {
		c := &serial.Config{Name: conf.P1.Device, Baud: conf.P1.Baudrate}
		input, err = serial.OpenPort(c)
		if err != nil {
			log.Fatal(err)
		}

	} else {
		input, err = os.Open(*testfile)
		if err != nil {
			log.Fatal(err)
		}
		if *ratelimit > 0 {
			input = dsmr4p1.RateLimit(input, time.Duration(*ratelimit)*time.Second)
		}
	}

	// We support different ways of encoding the information to be sent on the
	// mqtt bus.
	encodingMap := map[string]codec.Handle{
		"json":    new(codec.JsonHandle),
		"msgpack": new(codec.MsgpackHandle),
		"binc":    new(codec.BincHandle),
		"cbor":    new(codec.CborHandle),
	}
	h, ok := encodingMap[conf.Encoding]
	if !ok {
		log.Fatal("Unsupported encoding specified in configuration file: ", conf.Encoding)
	}

	// Check QoS value
	if !mqtt.ValidQoS(conf.Mqtt.QoS) {
		log.Fatal("Invalid QoS value specified in configuration file: ", conf.Mqtt.QoS)
	}

	// Ok, let's start reading
	ch := dsmr4p1.Poll(input)
	for telegram := range ch {
		r, err := telegram.Parse()

		telegramTimestamp, err := dsmr4p1.ParseTimestamp(r[conf.Timestamp.OBIS][0])
		if err != nil {
			fmt.Println("Error parsing timestamp:", err)
			continue // Could be a random fluke? For now just carry on...
		}

		for _, output := range conf.Outputs {
			message := make(map[string]interface{})
			outputTimestamp := telegramTimestamp
			incomplete := false
			for _, key := range output.Keys {
				if key.Type == "verbatim" {
					message[key.Name] = r[key.Identifier][0]
				} else {

					// We assume that the value is always in the last item
					valueIndex := len(r[key.Identifier]) - 1

					value, unit, err := dsmr4p1.ParseValueWithUnit(r[key.Identifier][valueIndex])
					if err != nil {
						log.Println("Error in parsing value.", err)
					}

					if len(r[key.Identifier]) > 1 {
						// There is an additional value, which in this case means there is a
						// specific timestamp for this value, which overrides the timestamp
						// read earlier.
						outputTimestamp, err = dsmr4p1.ParseTimestamp(r[key.Identifier][0])
						if err != nil {
							log.Println("Error parsing timestamp.", err)
							continue
						}
					}

					if key.Delta {
						originalValue := value
						if key.previousValid {
							value = originalValue - key.previousValue
						} else {
							key.previousValid = true
							incomplete = true
						}
						key.previousValue = originalValue
					}
					message[key.Name] = value
					message[key.UnitName] = unit
				}
			}
			if !incomplete {
				if outputTimestamp.After(output.lastTimestamp) {
					switch conf.Timestamp.Format {
					case "unix":
						message[conf.Timestamp.Key] = outputTimestamp.Unix()
					case "unixnano":
						message[conf.Timestamp.Key] = outputTimestamp.UnixNano()
					default:
						message[conf.Timestamp.Key] = outputTimestamp.Format(conf.Timestamp.Format)
					}

					b := make([]byte, 0, 1024)
					enc := codec.NewEncoderBytes(&b, h)
					err := enc.Encode(message)

					output.lastTimestamp = outputTimestamp

					// Publish a message.
					err = cli.Publish(&client.PublishOptions{
						QoS:       mqtt.QoS0,
						TopicName: []byte(output.Topic),
						Message:   b,
					})
					if err != nil {
						panic(err)
					}
				}
			}
		}
	}
	fmt.Println("Done. Exiting.")
}
