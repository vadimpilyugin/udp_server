package main

import (
  "github.com/vadimpilyugin/debug_print_go"
  "net"
)

func main() {
  printer.Debug("Hello, world!")
  // listen to incoming udp packets
  pc, err := net.ListenPacket("udp", "127.0.0.1:5553")
  if err != nil {
    printer.Fatal(err)
  }
  defer pc.Close()
  printer.Debug("Listening...")

  //simple read
  buffer := make([]byte, 1024)
  printer.Debug("Waiting for read...")
  pc.ReadFrom(buffer)

  printer.Debug(buffer)

}