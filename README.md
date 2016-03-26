# p1mqtt #
A simple but flexible program that can read data from a smart meter P1 port and publish it to an MQTT channel. You have (almost) complete control on how this data is encoded in the mqtt messages. For example, currently [json](http://json.org/), [msgpack](https://github.com/msgpack/msgpack), [binc](http://github.com/ugorji/binc), and [cbor](http://cbor.io/) are supported. You are also free to combine the smart meter measurements in one message for a single MQTT topic or to send out each measurement to a separate mqtt topic.

## Building & installing ##
First make sure you have setup your [Go](https://golang.org) environment. After having setup your $GOPATH you can do a

    go get github.com/mhe/p1mqtt

to install a p1mqtt binary in your $GOPATH/bin.

Alternatively you can clone this repository somewhere (for example in your $GOPATH/src) and install the dependencies

    cd p1mqtt
    go get ...

Then build it:

    go build .

Which should result in a binary in your working directory.

## Configuration ##
p1mqtt is configured through a small configuration file. This repository contains an example configuration file (p1mqtt-example.tom) that should be mostly self-explainatory. There are also a couple of commandline flags that can be set, see

    ./p1mqtt -h

## Experimenting ##
p1mqtt also supports reading data from a file, which is convenient to test your configuration. To generate such a file for testing simply store the output of the serial port somewhere, e.g.:

    dd if=/dev/ttyUSB0 of=testfile.bin

You can specify that a file should be used for input with the commandline flags, for example:

    ./p1mqtt -config p1mqtt.toml -testfile ./testfile.bin -ratelimit 10

The ratelimit flag allows you to simulate a smart meter somewhat by releasing a telegram every n (in this case, 10) seconds.

## Related projects ##

p1mqtt uses the [dsmr4p1](https://github.com/mhe/dsmr4p1) library for parsing the P1 telegrams.
