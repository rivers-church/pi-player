package main

import (
	"fmt"

	"github.com/17xande/keylogger"
)

func main() {
	path := "\"HBGIC Technology Co., Ltd. USB Keyboard Mouse\""
	fmt.Println("Creating new keylogger")
	kl := keylogger.NewKeyLogger(path)

	chans, err := kl.Read()
	if err != nil {
		panic(err)
	}

	for _, c := range chans {
		go listen(c)
	}

	for {

	}
}

func listen(c chan keylogger.InputEvent) {
	var ie keylogger.InputEvent
	for {
		ie = <-c
		fmt.Printf("%#v", ie)
	}
}
