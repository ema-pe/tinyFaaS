package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/OpenFogStack/tinyFaaS/pkg/docker"
	"github.com/OpenFogStack/tinyFaaS/pkg/manager"
	"github.com/google/uuid"
)

const (
	ConfigPort          = 8080
	RProxyConfigPort    = 8081
	RProxyListenAddress = ""

	// ShutdownTimeout is how long to wait for in-flight requests to the
	// management API before giving up and cleaning up anyway. Keep it below
	// the "docker stop" grace period (10s by default), otherwise the cleanup
	// may be cut short by SIGKILL.
	ShutdownTimeout = 5 * time.Second
)

type server struct {
	ms *manager.ManagementService
}

func main() {
	os.Exit(run())
}

func run() int {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("manager: ")

	ports := map[string]int{
		"http": 8000,
		"grpc": 9000,
	}

	for p := range ports {
		portstr := os.Getenv(strings.ToUpper(p) + "_PORT")

		if portstr == "" {
			continue
		}

		port, err := strconv.Atoi(portstr)

		if err != nil {
			log.Printf("invalid port for protocol %s: %s (must be an integer!)", p, err)
			return 1
		}

		if port < 0 {
			delete(ports, p)
			continue
		}

		if port > 65535 {
			log.Printf("invalid port for protocol %s: %s (must be an integer lower than 65535!)", p, err)
			return 1
		}

		ports[p] = port
	}

	// setting backend to docker
	id := uuid.New().String()

	// find backend
	backend, ok := os.LookupEnv("TF_BACKEND")

	if !ok {
		backend = "docker"
		log.Println("using default backend docker")
	}

	var tfBackend manager.Backend
	switch backend {
	case "docker":
		log.Println("using docker backend")
		tfBackend = docker.New(id)
	default:
		log.Printf("invalid backend %s", backend)
		return 1
	}

	ms := manager.New(
		id,
		RProxyListenAddress,
		ports,
		RProxyConfigPort,
		tfBackend,
	)

	rproxyArgs := []string{fmt.Sprintf("%s:%d", RProxyListenAddress, RProxyConfigPort)}

	for prot, port := range ports {
		rproxyArgs = append(rproxyArgs, fmt.Sprintf("%s:%s:%d", prot, RProxyListenAddress, port))
	}

	log.Println("rproxy args:", rproxyArgs)

	// unpack the rproxy binary in a temporary directory
	rProxyDir := path.Join(os.TempDir(), id)

	err := os.MkdirAll(rProxyDir, 0755)
	if err != nil {
		log.Print(err)
		return 1
	}

	err = os.WriteFile(path.Join(rProxyDir, "rproxy.bin"), RProxyBin, 0755)
	if err != nil {
		log.Print(err)
		return 1
	}

	defer os.RemoveAll(rProxyDir)

	c := exec.Command(path.Join(rProxyDir, "rproxy.bin"), rproxyArgs...)
	stdout, err := c.StdoutPipe()
	if err != nil {
		log.Print(err)
		return 1
	}

	stderr, err := c.StderrPipe()
	if err != nil {
		log.Print(err)
		return 1
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	if err := c.Start(); err != nil {
		log.Print(err)
		return 1
	}

	rproxy := c.Process

	log.Println("started rproxy")

	s := &server{ms: ms}

	// create handlers
	r := http.NewServeMux()
	r.HandleFunc("/upload", s.uploadHandler)
	r.HandleFunc("/delete", s.deleteHandler)
	r.HandleFunc("/list", s.listHandler)
	r.HandleFunc("/wipe", s.wipeHandler)
	r.HandleFunc("/logs", s.logsHandler)
	r.HandleFunc("/uploadURL", s.urlUploadHandler)

	srv := &http.Server{Addr: fmt.Sprintf(":%d", ConfigPort), Handler: r}

	// Handle SIGINT (Ctrl+C) and SIGTERM (issued by "docker stop" or "kill")
	// signals.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	// On a signal, gracefully shut down the HTTP server: this makes
	// ListenAndServe below return, so run() can clean up and return normally.
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)

		received := <-sig
		log.Printf("received signal %s", received)
		log.Println("shutting down HTTP server")

		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("error shutting down HTTP server: %s", err)
		}
	}()

	exitCode := 0

	log.Println("starting HTTP server")
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		// Graceful shutdown: wait for in-flight requests (or the timeout).
		<-shutdownDone
	} else {
		log.Print(err)
		exitCode = 1
	}

	// Clean up on both paths, so rproxy and function containers are not left
	// behind if the server fails to start.
	log.Println("stopping rproxy")
	if err := rproxy.Kill(); err != nil {
		log.Println(err)
	}
	// Wait for rproxy termination.
	_ = c.Wait()

	log.Println("stopping management service")
	if err := ms.Stop(); err != nil {
		log.Println(err)
	}

	return exitCode
}

func (s *server) uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// parse request
	d := struct {
		FunctionName    string   `json:"name"`
		FunctionEnv     string   `json:"env"`
		FunctionThreads int      `json:"threads"`
		FunctionZip     string   `json:"zip"`
		FunctionEnvs    []string `json:"envs"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&d)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}

	log.Println("got request to upload function: Name", d.FunctionName, "Env", d.FunctionEnv, "Threads", d.FunctionThreads, "Bytes", len(d.FunctionZip), "Envs", d.FunctionEnvs)

	envs := make(map[string]string)
	for _, e := range d.FunctionEnvs {
		k, v, ok := strings.Cut(e, "=")

		if !ok {
			log.Println("invalid env:", e)
			continue
		}

		envs[k] = v
	}

	res, err := s.ms.Upload(d.FunctionName, d.FunctionEnv, d.FunctionThreads, d.FunctionZip, envs)
	if err != nil {
		log.Println(err)
		if errors.Is(err, manager.ErrInvalidFunctionName) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, err)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// return success
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, res)
}

func (s *server) deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// parse request
	d := struct {
		FunctionName string `json:"name"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&d)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}

	log.Println("got request to delete function:", d.FunctionName)

	// delete function
	err = s.ms.Delete(d.FunctionName)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	// return success
	w.WriteHeader(http.StatusOK)
}

func (s *server) listHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	l := s.ms.List()

	// return success
	w.WriteHeader(http.StatusOK)
	for _, f := range l {
		fmt.Fprintf(w, "%s\n", f)
	}
}

func (s *server) wipeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := s.ms.Wipe()

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *server) logsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// parse request
	var logs io.Reader
	name := r.URL.Query().Get("name")

	if name == "" {
		l, err := s.ms.Logs()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Println(err)
			return
		}
		logs = l
	}

	if name != "" {
		l, err := s.ms.LogsFunction(name)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Println(err)
			return
		}
		logs = l
	}

	// return success
	w.WriteHeader(http.StatusOK)
	// w.Header().Set("Content-Type", "text/plain")
	_, err := io.Copy(w, logs)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}
}

func (s *server) urlUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// parse request
	d := struct {
		FunctionName    string   `json:"name"`
		FunctionEnv     string   `json:"env"`
		FunctionThreads int      `json:"threads"`
		FunctionURL     string   `json:"url"`
		FunctionEnvs    []string `json:"envs"`
		SubFolder       string   `json:"subfolder_path"`
	}{}

	err := json.NewDecoder(r.Body).Decode(&d)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}

	log.Println("got request to upload function:", d)

	envs := make(map[string]string)
	for _, e := range d.FunctionEnvs {
		k, v, ok := strings.Cut(e, "=")

		if !ok {
			log.Println("invalid env:", e)
			continue
		}

		envs[k] = v
	}

	res, err := s.ms.UrlUpload(d.FunctionName, d.FunctionEnv, d.FunctionThreads, d.FunctionURL, d.SubFolder, envs)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	// return success
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, res)
}
