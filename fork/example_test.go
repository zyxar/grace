package fork

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/zyxar/grace/sigutil"
)

func ExampleListen() {
	ln, err := Listen("tcp", "127.0.0.1:12345")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer ln.Close()
	srv := &http.Server{Addr: ln.Addr().String()}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%v\n", os.Getpid())
	})
	go func() { log.Fatal(srv.Serve(ln)) }()

	sigutil.Trap(func(s sigutil.Signal) {
		os.Exit(0)
	}, sigutil.SIGINT, sigutil.SIGTERM)
}

func ExampleReload() {
	ln, err := Listen("tcp", "127.0.0.1:12345")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer ln.Close()

	sigutil.Watch(func(s sigutil.Signal) {
		pid, err := Reload(ln)
		if err != nil {
			log.Println(err)
			return
		}
		log.Println("reloaded as", pid)
		os.Exit(0)
	}, sigutil.SIGHUP)

	sigutil.Trap(func(s sigutil.Signal) {
		os.Exit(0)
	}, sigutil.SIGINT, sigutil.SIGTERM)
}
