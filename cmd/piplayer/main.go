package main

import (
	"flag"
	"log"
	"os"

	"github.com/17xande/keylogger"
	piplayer "github.com/17xande/pi-player/pkg/piplayer"
)

func main() {
	addr := flag.String("addr", ":8080", "The addr of the application.")
	test := flag.String("test", "", "send \"mac\", \"linux\", or \"web\" to test the code on mac or linux or to test only the web interface.")
	debug := flag.Bool("debug", false, "print extra information for debugging.")
	dlv := flag.Bool("dlv", false, "Let the program know if delve is being used to debug so the application directory can be changed.")
	flag.Parse()

	if *dlv {
		os.Chdir("../..")
	}

	var conf piplayer.Config
	if err := conf.Load(""); err != nil {
		log.Fatal("Error loading config.", err)
	}

	dbg := *debug || conf.Debug

	if dbg {
		log.Println("Debug mode enabled")
		log.Println("Config file -> Directory: ", conf)
	}

	a := piplayer.NewAPIHandler(dbg, test)
	kl := keylogger.NewKeyLogger(conf.Remote.Name)
	p := piplayer.NewPlayer(&a, &conf, kl)
	p.Server = piplayer.NewServer(p, *addr)

	defer p.CleanUp()

	// Start the browser
	// We have to start it async because the code has
	// to carry on, so that the server comes online.
	go p.FirstRun()

	piplayer.Start(p.Server)
}
