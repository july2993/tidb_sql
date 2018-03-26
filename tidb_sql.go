// https://dev.mysql.com/doc/internals/en/client-server-protocol.html
// https://dev.mysql.com/doc/dev/mysql-server/latest/PAGE_PROTOCOL.html

package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/july2993/tidb_sql/mysql"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/tcpassembly"
	"github.com/google/gopacket/tcpassembly/tcpreader"
)

var iface = flag.String("i", "eth0", "Interface to get packets from")
var tidbPort = flag.Int("port", 4000, "port of tidb-server or mysql")

func init() {

	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type mysqlStreamFactory struct {
	source map[string]*mysqlStream
}

type packet struct {
	seq     uint8
	payload []byte
}

type mysqlStream struct {
	net, transport gopacket.Flow
	r              tcpreader.ReaderStream

	stmtID2query map[uint32]*mysql.Stmt

	packets chan *packet
}

func (m *mysqlStreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	mstream := &mysqlStream{
		net:          net,
		transport:    transport,
		r:            tcpreader.NewReaderStream(),
		stmtID2query: make(map[uint32]*mysql.Stmt),
		packets:      make(chan *packet, 1024),
	}

	log.Println("new stream ", net, transport)
	go mstream.readPackets()

	key := fmt.Sprintf("%v:%v", net, transport)
	rev_key := fmt.Sprintf("%v:%v", net.Reverse(), transport.Reverse())

	// server to client stream
	if transport.Src().String() == strconv.Itoa(*tidbPort) {
		if client, ok := m.source[rev_key]; ok {
			log.Println("run ", rev_key)
			go client.runClient(mstream.packets)
			delete(m.source, rev_key)
		} else {
			// wait client stream
			m.source[key] = mstream
		}
	} else { // client to server stream
		if server, ok := m.source[rev_key]; ok {
			log.Println("run ", key)
			go mstream.runClient(server.packets)
			delete(m.source, rev_key)
		} else {
			// wait server stream
			m.source[key] = mstream
		}
	}

	// ReaderStream implements tcpassembly.Stream, so we can return a pointer to it.
	return &mstream.r
}

func (m *mysqlStream) readPackets() {
	buf := bufio.NewReader(&m.r)
	for {
		seq, pk, err := mysql.ReadPacket(buf)
		if err == io.EOF {
			log.Println(m.net, m.transport, " leave")
			close(m.packets)
			return
		} else if err != nil {
			log.Println("Error reading stream", m.net, m.transport, ":", err)
			close(m.packets)
		} else {
			// log.Println("Received package from stream", m.net, m.transport, " seq: ", seq, " pk:", pk)
		}

		m.packets <- &packet{seq: seq, payload: pk}
	}

}

// for simplicy, does'n parse server response according to request
// just skip to the first response packet to try get response stmt_id now
func skip2Seq(srv chan *packet, seq uint8) *packet {
	for {
		select {
		case pk, ok := <-srv:
			if !ok {
				return nil
			}
			if pk.seq == seq {
				return pk
			}
		case <-time.After(5 * time.Second):
			return nil
		}
	}
}

func (m *mysqlStream) handlePacket(seq uint8, payload []byte, srvPackets chan *packet) {
	// text protocol command can just print it out
	// https://dev.mysql.com/doc/internals/en/text-protocol.html
	srvPK := skip2Seq(srvPackets, seq+1)
	switch payload[0] {
	// 131, 141 for handkshake
	// https://dev.mysql.com/doc/internals/en/connection-phase-packets.html#packet-Protocol::Handshake
	case 131, 141:
	// some old client may still use this, print it in sql query way
	case mysql.COM_INIT_DB:
		fmt.Printf("use %s;\n", payload[1:])
	case mysql.COM_DROP_DB:
		fmt.Printf("DROP DATABASE %s;\n", payload[1:])
	case mysql.COM_CREATE_DB:
		fmt.Printf("CREATE DATABASE %s;\n", payload[1:])
	// just print the query
	case mysql.COM_QUERY:
		fmt.Printf("%s;\n", payload[1:])

	// prepare statements
	// https://dev.mysql.com/doc/internals/en/prepared-statements.html
	case mysql.COM_STMT_PREPARE:
		// find the return stmt_id, so we can know which prepare stmt execute later
		if srvPK == nil {
			log.Println("can't find resp packet from prepare")
			return
		}

		// https://dev.mysql.com/doc/internals/en/com-stmt-prepare-response.html#packet-COM_STMT_PREPARE_OK
		if srvPK.payload[0] != 0 {
			log.Println("prepare fail")
			return
		}

		stmtID := binary.LittleEndian.Uint32(srvPK.payload[1:5])
		stmt := &mysql.Stmt{
			ID:    stmtID,
			Query: string(payload[1:]),
		}
		m.stmtID2query[stmtID] = stmt
		stmt.Columns = binary.LittleEndian.Uint16(srvPK.payload[5:7])
		stmt.Params = binary.LittleEndian.Uint16(srvPK.payload[7:9])
		stmt.Args = make([]interface{}, stmt.Params)

		log.Println("prepare stmt: ", *stmt)
	case mysql.COM_STMT_SEND_LONG_DATA:
		// https://dev.mysql.com/doc/internals/en/com-stmt-send-long-data.html
		stmtID := binary.LittleEndian.Uint32(payload[1:5])
		paramId := binary.LittleEndian.Uint16(payload[5:7])
		stmt, ok := m.stmtID2query[stmtID]
		if !ok {
			return
		}
		if paramId >= stmt.Params {
			return
		}

		if stmt.Args[paramId] == nil {
			stmt.Args[paramId] = payload[7:]
		} else {
			if b, ok := stmt.Args[paramId].([]byte); ok {
				b = append(b, payload[7:]...)
				stmt.Args[paramId] = b
			}
		}
	case mysql.COM_STMT_RESET:
		// https://dev.mysql.com/doc/internals/en/com-stmt-reset.html
		stmtID := binary.LittleEndian.Uint32(payload[1:5])
		stmt, ok := m.stmtID2query[stmtID]
		if !ok {
			return
		}
		stmt.Args = make([]interface{}, stmt.Params)

	case mysql.COM_STMT_EXECUTE:
		// https://dev.mysql.com/doc/internals/en/com-stmt-execute.html
		idx := 1
		stmtID := binary.LittleEndian.Uint32(payload[idx : idx+4])
		idx += 4
		var stmt *mysql.Stmt
		var ok bool
		if stmt, ok = m.stmtID2query[stmtID]; ok == false {
			log.Println("not found stmt id query: ", stmtID)
			return
		}
		fmt.Printf("# exec prepare stmt:  %s;\n", stmt.Query)
		// parse params
		flags := payload[idx]
		_ = flags
		idx += 1
		// skip iterater_count alwasy 1
		_ = binary.LittleEndian.Uint32(payload[idx : idx+4])
		idx += 4
		if stmt.Params > 0 {
			len := int((stmt.Params + 7) / 8)
			nullBitmap := payload[idx : idx+len]
			idx += len

			newParamsBoundFlag := payload[idx]
			idx += 1

			var paramTypes []byte
			var paramValues []byte
			if newParamsBoundFlag == 1 {
				paramTypes = payload[idx : idx+int(stmt.Params)*2]
				idx += int(stmt.Params) * 2
				paramValues = payload[idx:]
			}
			err := stmt.BindStmtArgs(nullBitmap, paramTypes, paramValues)
			if err != nil {
				log.Println("bind args err: ", err)
				return
			}
		}
		// log.Println("exec smmt: ", *stmt)
		fmt.Println("# binary exec a prepare stmt rewrite it like: ")
		fmt.Println(string(stmt.WriteToText()))
	case mysql.COM_STMT_CLOSE:
		// https://dev.mysql.com/doc/internals/en/com-stmt-close.html
		// delete the stmt will not be use any more
		stmtID := binary.LittleEndian.Uint32(payload[1:5])
		delete(m.stmtID2query, stmtID)
	default:
	}
}

func (m *mysqlStream) runClient(srv chan *packet) {
	for packet := range m.packets {
		m.handlePacket(packet.seq, packet.payload, srv)
	}
}

func main() {
	flag.Parse()

	var handle *pcap.Handle
	var err error

	if handle, err = pcap.OpenLive(*iface, 1600, true, pcap.BlockForever); err != nil {
		panic(err)
	} else if err := handle.SetBPFFilter(fmt.Sprintf("tcp and port %d", *tidbPort)); err != nil {
		panic(err)
	}

	// Set up assembly
	streamFactory := &mysqlStreamFactory{source: make(map[string]*mysqlStream)}
	streamPool := tcpassembly.NewStreamPool(streamFactory)
	assembler := tcpassembly.NewAssembler(streamPool)

	// Read in packets, pass to assembler.
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packets := packetSource.Packets()
	ticker := time.Tick(time.Minute)

	for {
		select {
		case packet := <-packets:
			if packet == nil {
				return
			}
			// log.Println(packet)
			if packet.NetworkLayer() == nil || packet.TransportLayer() == nil || packet.TransportLayer().LayerType() != layers.LayerTypeTCP {
				log.Println("Unusable packet")
				continue
			}
			tcp := packet.TransportLayer().(*layers.TCP)
			assembler.AssembleWithTimestamp(packet.NetworkLayer().NetworkFlow(), tcp, packet.Metadata().Timestamp)
		case <-ticker:
			// Every Minus, flush connections that haven't seen activity in the past 2 Minute.
			assembler.FlushOlderThan(time.Now().Add(time.Minute * -2))
		}
	}

}
