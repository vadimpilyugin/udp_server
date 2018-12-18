package main

import (
  "github.com/vadimpilyugin/debug_print_go"
  "net"
  "os"
)

const (
  MTU = 1500
)

func usage() {
  println("Usage: main [ADDR]")
  os.Exit(1)
}

func main() {
  if len(os.Args) < 2 {
    usage()
  }
  printer.Debug("Hello, world!")

  addr := os.Args[1]

  // listen to incoming udp packets
  pc, err := net.ListenPacket("udp", addr)
  if err != nil {
    printer.Fatal(err)
  }
  defer pc.Close()
  printer.Debug("Listening...")

  //simple read
  buffer := make([]byte, MTU)
  printer.Debug("Waiting for read...")
  pc.ReadFrom(buffer)

  printer.Debug(buffer)

}