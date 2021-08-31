package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ferama/vipien/pkg/iface"
	"github.com/ferama/vipien/pkg/util"
	"github.com/gorilla/websocket"
	"github.com/songgao/water"
	"github.com/songgao/water/waterutil"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:    1500,
	WriteBufferSize:   1500,
	EnableCompression: true,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Server struct {
	registry *Registry
	tun      *water.Interface
}

func New(iface *iface.IFace) *Server {
	s := &Server{
		tun:      iface.Tun,
		registry: NewRegistry(),
	}

	return s
}

func (s *Server) tun2ws() {
	buffer := make([]byte, 1500)

	for {
		n, err := s.tun.Read(buffer)
		if err != nil || err == io.EOF || n == 0 {
			continue
		}
		b := buffer[:n]
		if !waterutil.IsIPv4(b) {
			continue
		}

		srcAddr, dstAddr := util.GetAddr(b)
		if srcAddr == "" || dstAddr == "" {
			continue
		}

		// for _, v := range hub.clients {
		// 	v.WriteMessage(websocket.BinaryMessage, buffer)
		// }
		key := fmt.Sprintf("%v->%v", dstAddr, srcAddr)
		log.Println(key)
		if conn, err := s.registry.GetByKey(key); err == nil {
			conn.WriteMessage(websocket.BinaryMessage, buffer)
		}
	}
}

func (s *Server) ws2tun(ws *websocket.Conn) {
	key := ""
	defer func() {
		if key != "" {
			s.registry.Delete(key)
		}
	}()
	for {
		ws.SetReadDeadline(time.Now().Add(time.Duration(30) * time.Second))
		_, b, err := ws.ReadMessage()
		if err != nil || err == io.EOF {
			break
		}
		if !waterutil.IsIPv4(b) {
			continue
		}
		srcAddr, dstAddr := util.GetAddr(b)
		if srcAddr == "" || dstAddr == "" {
			continue
		}
		key = fmt.Sprintf("%v->%v", srcAddr, dstAddr)
		s.registry.Add(key, ws)
		s.tun.Write(b[:])
	}
}

func (s *Server) Run(addr string) {
	go s.tun2ws()

	http.HandleFunc("/ip", func(w http.ResponseWriter, req *http.Request) {
		ip := req.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = strings.Split(req.RemoteAddr, ":")[0]
		}
		resp := fmt.Sprintf("%v", ip)
		io.WriteString(w, resp)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)

		if err != nil {
			return
		}
		s.ws2tun(ws)
	})

	log.Println("Listening on", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
