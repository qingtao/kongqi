// +build windows
package main

import (
	"fmt"
	"github.com/tarm/serial"
	"testing"
	"time"
)

var config = &serial.Config{Name: "COM3", Baud: 57600}

func TestCom(t *testing.T) {
	sch := make(chan string, 10)
	ech := make(chan error, 10)

	go func() {
		for i := 0; i < 3; i++ {
			select {
			case v := <-sch:
				fmt.Printf("%d---%s\n", i, time.Now())
				fmt.Printf("vch: %s\n", v)
			case e := <-ech:
				fmt.Printf("%d---%s\n", i, time.Now())
				fmt.Printf("ech: %s\n", e)
			}
		}
	}()

	s, err := serial.OpenPort(config)
	if err != nil {
		t.Fatal(err)
	}
	ReadLine(s, sch, ech)
}
