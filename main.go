package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	logLevelAll   = "all"
	logLevelDebug = "debug"
	logLevelInfo  = "info"
	logLevelWarn  = "warn"
	logLevelError = "error"
	logLevelNone  = "none"
)

var (
	availableLogLevels = strings.Join([]string{
		logLevelAll,
		logLevelDebug,
		logLevelInfo,
		logLevelWarn,
		logLevelError,
		logLevelNone,
	}, ", ")
)

func main() {
	cmd := &cobra.Command{
		Use:   "configmap-to-disk",
		Args:  cobra.ArbitraryArgs,
		Short: "Watch ConfigMaps in the API and write them to disk",
		Long:  "",
	}
	var kubeconfig string
	cmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to kubeconfig.")
	var namespace string
	cmd.PersistentFlags().StringVar(&namespace, "namespace", "", "The namespace to watch.")
	var path string
	cmd.PersistentFlags().StringVar(&path, "path", "", "Where to write the file.")
	var name string
	cmd.PersistentFlags().StringVar(&name, "name", "", "The ConfigMap name.")
	var key string
	cmd.PersistentFlags().StringVar(&key, "key", "", "The ConfigMap key to read.")
	var listen string
	cmd.PersistentFlags().StringVar(&listen, "listen", ":8080", "The address at which to listen for health and metrics.")
	var logLevel string
	cmd.PersistentFlags().StringVar(&logLevel, "log-level", logLevelInfo, fmt.Sprintf("Log level to use. Possible values: %s", availableLogLevels))
	var syncOneTime bool
	cmd.PersistentFlags().BoolVar(&syncOneTime, "one-time", false, "Syncs the configmap to disk a single time and exits.")

	var c kubernetes.Interface
	var logger log.Logger
	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		logger = log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
		switch logLevel {
		case logLevelAll:
			logger = level.NewFilter(logger, level.AllowAll())
		case logLevelDebug:
			logger = level.NewFilter(logger, level.AllowDebug())
		case logLevelInfo:
			logger = level.NewFilter(logger, level.AllowInfo())
		case logLevelWarn:
			logger = level.NewFilter(logger, level.AllowWarn())
		case logLevelError:
			logger = level.NewFilter(logger, level.AllowError())
		case logLevelNone:
			logger = level.NewFilter(logger, level.AllowNone())
		default:
			return fmt.Errorf("log level %v unknown; possible values are: %s", logLevel, availableLogLevels)
		}
		logger = log.With(logger, "ts", log.DefaultTimestampUTC)
		logger = log.With(logger, "caller", log.DefaultCaller)

		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return fmt.Errorf("create Kubernetes config: %w", err)
		}
		c = kubernetes.NewForConfigOrDie(config)

		return nil
	}

	// Determine whether to run once or run continuously
	if !syncOneTime {
		cmd.RunE = runCmd(&c, &listen, &namespace, &path, &name, &key, &logger)
	}
	cmd.RunE = runCmdOneTime(&c, &namespace, &path, &name, &key, &logger)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func runCmd(c *kubernetes.Interface, listen, namespace, path, name, key *string, logger *log.Logger) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		r := prometheus.NewRegistry()
		r.MustRegister(
			prometheus.NewGoCollector(),
			prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		)

		c := newController(*c, *namespace, *path, *name, *key, *logger, r)

		var g run.Group
		{
			// Run the HTTP server.
			mux := http.NewServeMux()
			mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			mux.Handle("/metrics", promhttp.HandlerFor(r, promhttp.HandlerOpts{}))
			l, err := net.Listen("tcp", *listen)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", *listen, err)
			}

			g.Add(func() error {
				if err := http.Serve(l, mux); err != nil && err != http.ErrServerClosed {
					return fmt.Errorf("error: server exited unexpectedly: %w", err)
				}
				return nil
			}, func(error) {
				l.Close()
			})
		}

		{
			stop := make(chan struct{})
			g.Add(func() error {
				if err := c.run(stop); err != nil {
					return fmt.Errorf("controller quit unexpectedly: %w", err)
				}
				return nil
			}, func(error) {
				close(stop)
			})
		}

		return g.Run()
	}
}

func runCmdOneTime(c *kubernetes.Interface, namespace, path, name, key *string, logger *log.Logger) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		level.Info(*logger).Log("msg", "Runing configmap-to-disk in one time mode.")
		return runOneTime(*c, *namespace, *path, *name, *key, *logger)
	}
}
