package main

import (
    "net/http"
    "sync"
    "flag"
    "os"
    "time"
    "github.com/op/go-logging"
)

const (
    usage = "webserver [options]\n"
)

var (
    sockpath = ""
    help = false
    mu sync.Mutex
    count int
    listen string
    listentls string
    cert string
    key string
    colour bool
    documentRoot string
    sleepTime time.Duration = time.Second * 10
    holdtime int
    log *logging.Logger
    debug = false
)

func init() {
    flag.BoolVar(&help, "h", false, "Print help")
    flag.StringVar(&listen, "l", "0.0.0.0:80", "Listen address for http (blank for none)")
    flag.StringVar(&listentls, "t", "0.0.0.0:443", "Listen address for https (blank for none)")
    flag.StringVar(&cert, "c", "cert.pem", "Path to cert.pem file")
    flag.StringVar(&key, "k", "key.pem", "Path to key.pem file")
    flag.StringVar(&documentRoot, "r", "/var/www/html", "Document root to serve from")
    flag.IntVar(&holdtime, "H", 0, "Hold time on requests - ie. wait before responding")
    flag.Parse()

    if help {
        flag.PrintDefaults()
        os.Exit(1)
    }

    format := logging.MustStringFormatter(
        `%{time:2006-01-02 15:04:05.000-0700} %{level} [%{shortfile}] %{message}`,
        )
    stderrBackend := logging.NewLogBackend(os.Stderr, "", 0)
    stderrFormatter := logging.NewBackendFormatter(stderrBackend, format)
    stderrBackendLevelled := logging.AddModuleLevel(stderrFormatter)
    logging.SetBackend(stderrBackendLevelled)
    if debug {
        stderrBackendLevelled.SetLevel(logging.DEBUG, "webserver")
    } else {
        stderrBackendLevelled.SetLevel(logging.INFO, "webserver")
    }
    log = logging.MustGetLogger("webserver")
}

/*
 * The status response writer, to capture response status for logging.
 */

type statusResponseWriter struct {
    http.ResponseWriter
    statusCode int
}

func NewStatusResponseWriter(w http.ResponseWriter) *statusResponseWriter {
    return &statusResponseWriter{w, http.StatusOK}
}

func (sw statusResponseWriter) WriteHeader(code int) {
    sw.statusCode = code
    sw.ResponseWriter.WriteHeader(code)
}

/*
 * This is a logging wrapper for all of the http handlers, so we don't
 * have to copy and paste logging statements into each handler.
 */
func logHttp(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
    return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := NewStatusResponseWriter(w)
        log.Info(r.Method, " ", r.URL, " ", r.Proto)
        // Sleep for holdtime before responding, if it's non-zero
        if holdtime != 0 {
            log.Infof("holding for %d seconds\n", holdtime)
            time.Sleep(time.Second*time.Duration(holdtime))
        }
        handler(sw, r)
        log.Info("back from handler")
		dur := time.Since(start)
        took := float64(dur) / float64(time.Millisecond)
		log.Infof("    --> %d %s - %0.3fms\n", sw.statusCode, http.StatusText(sw.statusCode), took)
    }
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {
    f := http.FileServer(http.Dir(documentRoot))
    sw := NewStatusResponseWriter(w)
    f.ServeHTTP(*sw, r)
}

func main() {

    var ch chan string
    ch = make(chan string)
    mux := http.NewServeMux()
    mux.HandleFunc("/", logHttp(defaultHandler))

    serving_http := false
    serving_https := false

    if listen != "" {
        server := &http.Server {
            Addr:   listen,
            Handler: mux,
            //ErrorLog: log,
        }
        serving_http = true
        go func(ch chan string) {
            log.Infof("Starting server on %s...\n", listen)
            err := server.ListenAndServe()
            if err != nil {
                log.Error(err)
            }
            ch <- "http"
        }(ch)
    }

    if listentls != "" {
        tls_server := &http.Server {
            Addr: listentls,
            Handler: mux,
            //ErrorLog: log,
        }
        serving_https = true
        go func(ch chan string) {
            log.Infof("Starting TLS server on %s...\n", listentls)
            tls_server.ListenAndServeTLS(cert, key)
        }(ch)
        ch <- "https"
    }

    // Wait for goroutines to return
    for {
        port := <- ch
        if (port == "http") {
            serving_http = false
        } else {
            serving_https = false
        }
        if !serving_http && !serving_https {
            break
        }
    }
}
