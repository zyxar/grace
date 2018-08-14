package fork

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/zyxar/grace/sigutil"
)

func ExampleGetArgs() {
	args := GetArgs(nil, func(arg string) bool {
		return arg == "daemon"
	})
	program, _ := os.Executable() // go1.8+
	if err := Daemonize(program, &Option{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
	}, args...); err != nil {
		panic(err)
	}
}

func ExampleDaemonize() {
	getArgs := func() []string {
		if flag.NFlag() == 0 {
			return flag.Args()
		}
		flag.Set("daemon", "false")
		args := make([]string, 0, flag.NFlag()+flag.NArg())
		flag.Visit(func(f *flag.Flag) {
			args = append(args, "-"+f.Name+"="+f.Value.String())
		})
		return append(args, flag.Args()...)
	}

	var daemon = flag.Bool("daemon", false, "enable daemon mode")
	flag.Parse()
	if *daemon {
		stderr, err := os.OpenFile("stderr.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer stderr.Close()
		program, _ := os.Executable() // go1.8+
		if err = Daemonize(program, &Option{
			Stdout: os.Stdout, Stderr: stderr,
		}, getArgs()...); err != nil {
			panic(err)
		}
		return
	}

	if os.Getppid() == 1 { // process parent is init, or uses env variable
		os.Stdout.Close()
		defer os.Stderr.Close()
		os.Stdin.Close()
		log.Println("std fds closed in daemon mode")
	}

	sigutil.Trap(func(s sigutil.Signal) {
		os.Exit(0)
	}, sigutil.SIGINT, sigutil.SIGTERM)
}

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

func ExampleReload_multiple() {
	ln, err := Listen("tcp", "127.0.0.1:12345")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer ln.Close()

	ln1, err := Listen("tcp", "127.0.0.1:12346")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer ln1.Close()

	ln2, err := Listen("tcp", "127.0.0.1:12347")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer ln2.Close()

	srv := &http.Server{Addr: ln.Addr().String()}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "1:%v\n", os.Getpid())
	})
	go func() { log.Fatal(srv.Serve(ln)) }()

	srv1 := &http.Server{Addr: ln1.Addr().String()}
	srv1.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "2:%v\n", os.Getpid())
	})
	go func() { log.Fatal(srv1.Serve(ln1)) }()

	srv2 := &http.Server{Addr: ln2.Addr().String()}
	srv2.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "3:%v\n", os.Getpid())
	})
	go func() { log.Fatal(srv2.Serve(ln2)) }()

	sigutil.Watch(func(sigutil.Signal) {
		pid, err := Reload(ln, ln1, ln2)
		if err != nil {
			log.Println(err)
			return
		}
		log.Println("reloaded as", pid)
		os.Exit(0)
	}, sigutil.SIGHUP)

	sigutil.Trap(func(s sigutil.Signal) {
		log.Println("killed by", s)
		os.Exit(0)
	}, sigutil.SIGINT, sigutil.SIGTERM)
}
