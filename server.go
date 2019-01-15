package main

import (
  "github.com/vadimpilyugin/debug_print_go"
  "net"
  "os"
  "fmt"
  "time"
  "io"
  "encoding/binary"
)

const (
  SMBUF = 256
  WAITFOR = 3
  DO_RETRANSMIT = "Do another retransmission"
  READY = "Ready"
  FILE_RECEIVED = "File received"
  STATS = "Stats?"
  LF = '\n'
)
const (
  serverPortTcp = "8080"
  serverPortUdp = "8687"
)

type FilePart struct {
  Filename string
  PartNo int64
  NParts int64
  FilePart []byte
}

type CombinedFile struct {
  NParts int64
  RecvParts int64
  Filename string
  RecvStarted time.Time
  Parts [][]byte
  Content []byte
}

const (
  MTU = 1500
  MAX_LEN = 2 << 16
  BUFLEN = 4096
  MAX_FN_LEN = 20
  LEN_ERR = "Filename is too long"
  FN_L = 1
  INDEX_LEN = 8
  HEADER_LEN = FN_L + MAX_FN_LEN + 2 * INDEX_LEN
)

func (cf *CombinedFile) content() []byte {
  if cf.Content != nil {
    return cf.Content
  }
  buf := make([]byte, 0, BUFLEN)
  for _,part := range cf.Parts {
    buf = append(buf, part...)
  }
  cf.Content = buf
  return buf
}

func insertPart(filesParts map[string]*CombinedFile, fp *FilePart) *CombinedFile {
  if cf, found := filesParts[fp.Filename]; found {
    if cf.Parts[fp.PartNo] != nil {
      // retransmission of already received part
      return nil
    }
    cf.Parts[fp.PartNo] = fp.FilePart
    cf.RecvParts++
    // rp := filesParts[fp.Filename].RecvParts
    // np := filesParts[fp.Filename].NParts
    // printer.Note(
    //   fmt.Sprintf("%d / %d (%.2f %%) [%d left]", rp, np, float64(rp) / float64(np) * 100, np-rp), 
    //   fp.Filename,
    // )
    if cf.RecvParts == cf.NParts {
      return cf
    } else {
      return nil
    }
  } else {
    cf = &CombinedFile{
      NParts: fp.NParts,
      RecvParts: 0,
      Filename: fp.Filename,
      Parts: make([][]byte, fp.NParts),
      RecvStarted: time.Now(),
    }
    filesParts[fp.Filename] = cf
    // recursion will end, because filesParts now contains fp.Filename
    return insertPart(filesParts, fp)
  }
}

func (fp *FilePart) UnmarshalBinary(data []byte) error {
  fnLen := data[0]
  fp.Filename = string(data[1:fnLen+1])
  fp.PartNo, _ = binary.Varint(data[MAX_FN_LEN+1:MAX_FN_LEN+1+8])
  fp.NParts, _ = binary.Varint(data[MAX_FN_LEN+1+8:MAX_FN_LEN+1+8+8])
  fp.FilePart = data[HEADER_LEN:]
  return nil
}

func handleRecv(pc net.PacketConn, parts chan *FilePart) {
  //simple read
  buffer := make([]byte, MAX_LEN)
  for {
    n, _, err := pc.ReadFrom(buffer)
    if err != nil {
      printer.Fatal(err)
    }

    fp := &FilePart{}
    err = fp.UnmarshalBinary(buffer[:n])
    if err != nil {
      printer.Fatal(err)
    }
    parts <- fp
  }
}

func readMsg(c net.Conn) []byte {
  buffer := make([]byte, SMBUF)
  n, err := c.Read(buffer)
  if err != nil && err != io.EOF {
    printer.Fatal(err)
  } else if err == io.EOF {
    printer.Fatal(err, "Client exited")
  }
  if n > 0 {
    printer.Debug(buffer[:n-1], "--- client")
  }
  return buffer[:n]
}

func sendMsg(c net.Conn, msg string) {
  _, err := c.Write([]byte(msg + "\n"))
  if err != nil {
    printer.Fatal(err)
  }
  printer.Debug(msg, "--- me")
}

func readCommand(c net.Conn, received chan string) {
  var command []byte

  for {
    for _, ch := range readMsg(c) {
      if ch == LF {
        received <- string(command)
        command = []byte("")
      } else {
        command = append(command, ch)
      }
    }
  }
}

func printReceived(filesParts map[string]*CombinedFile) {
  for fn := range filesParts {
    rp := filesParts[fn].RecvParts
    np := filesParts[fn].NParts
    printer.Note(
      fmt.Sprintf("%d / %d (%.2f %%) [%d left]", rp, np, float64(rp) / float64(np) * 100, np-rp), 
      fn,
    )
  }
}

func testSeries(pc net.PacketConn, c net.Conn, received chan string, parts chan *FilePart) {
  var filesParts map[string]*CombinedFile
  var timer *time.Timer

  for {
    if msg := <-received; msg == READY {
      filesParts = make(map[string]*CombinedFile)
      sendMsg(c, READY)
      timer = time.NewTimer(WAITFOR * time.Second)
    }
OuterLoop:
    for {
      select {
        case <-timer.C:
          printReceived(filesParts)
          sendMsg(c, DO_RETRANSMIT)
          timer.Reset(WAITFOR * time.Second)
        case fp := <-parts:
          cf := insertPart(filesParts, fp)
          if cf != nil {
            timer.Stop()
            sendMsg(c, FILE_RECEIVED)
            if msg := <-received; msg == STATS {
              content := cf.content()
              fileSize := len(content)
              timeTaken := time.Since(cf.RecvStarted).Seconds()
              speed := float64(fileSize * 8) / 1000 / timeTaken

              printer.Note(cf.Filename,"--- file received")
              printer.Note(fileSize, "--- length (bytes)")
              printer.Note(timeTaken, "--- time taken (s)")
              printer.Note(speed, "--- mean speed (kbps)")

              sendMsg(c, fmt.Sprintf("%s,%d,%f,%f", cf.Filename, fileSize, timeTaken, speed))
              printReceived(filesParts)
              break OuterLoop
            }
          }
          timer.Reset(WAITFOR * time.Second)
      }
    }
  }

}

func usage() {
  println("Usage: server [ADDR]")
  os.Exit(1)
}

func main() {
  if len(os.Args) < 2 {
    usage()
  }

  serverAddr := os.Args[1]

  printer.Debug("Server started on "+serverAddr)

  // listen to incoming tcp connections
  l, err := net.Listen("tcp", serverAddr+":"+serverPortTcp)
  if err != nil {
    printer.Fatal(err)
  }
  defer l.Close()

  c, err := l.Accept()
  if err != nil {
    printer.Fatal(err)
  }
  
  // listen to incoming udp packets
  pc, err := net.ListenPacket("udp", serverAddr+":"+serverPortUdp)
  if err != nil {
    printer.Fatal(err)
  }
  defer pc.Close()

  received := make(chan string)
  go readCommand(c, received)

  parts := make(chan *FilePart)
  go handleRecv(pc, parts)

  testSeries(pc, c, received, parts)
}