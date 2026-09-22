package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	piplayer "github.com/17xande/pi-player/pkg/piplayer"
)

//go:embed pkg/piplayer/assets
var statAssets embed.FS

//go:embed pkg/piplayer/templates
var statTemplates embed.FS

var version = "dev"

func main() {
	addr := flag.String("addr", ":8080", "The addr of the application.")
	test := flag.String("test", "", "send \"mac\", \"linux\", or \"web\" to test the code on mac or linux or to test only the web interface.")
	debug := flag.Bool("debug", false, "print extra information for debugging.")
	ver := flag.Bool("version", false, "print version and exit.")
	flag.Parse()

	if *ver {
		fmt.Println(version)
		os.Exit(0)
	}

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	conf, err := piplayer.ConfigLoad(statAssets)
	if err != nil {
		log.Printf("Current directory: %s\n", exPath)
		log.Fatalf("Error loading config.\n%v", err)
	}

	if *debug || conf.Debug {
		conf.SetDebug(true)
	}
	piplayer.SetDebugLogging(conf.DebugEnabled())

	a, err := piplayer.NewAPIHandler(test, statAssets, statTemplates)
	if err != nil {
		log.Fatalf("Error setting up the web interface.\n%v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	p := piplayer.NewPlayer(ctx, &a, conf)
	p.Server = piplayer.NewServer(p, *addr)

	// Start the browser
	// We have to start it async because the code has
	// to carry on, so that the server comes online.
	go p.FirstRun()

	err = piplayer.Run(ctx, p)
	p.Close()
	if err != nil {
		log.Fatalf("server error: %v", err)
	}
}
