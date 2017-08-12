package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("Starting echo server")
	socketFile := "/tmp/go.sock"
	ln, err := net.Listen("unix", socketFile)
	if err != nil {
		log.Fatal("Listen error: ", err)
	} else {
		log.Println("Created socket at ", socketFile)
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func(ln net.Listener, c chan os.Signal) {
		sig := <-c
		log.Printf("Caught signal %s: shutting down.", sig)
		ln.Close()
		os.Exit(0)
	}(ln, sigc)

	for {
		log.Println("about to listen")
		_, err := ln.Accept()
		log.Println("We made it past accept")
		if err != nil {
			log.Fatal("Accept error: ", err)
		}

	}
}