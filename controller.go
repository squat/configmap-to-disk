package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type controller struct {
	client   kubernetes.Interface
	path     string
	key      string
	informer cache.SharedIndexInformer
	logger   log.Logger

	reconcileAttempts prometheus.Counter
	reconcileErrors   prometheus.Counter
}

func newController(client kubernetes.Interface, namespace, path, name, key string, logger log.Logger, reg prometheus.Registerer) *controller {
	pi := v1informers.NewFilteredConfigMapInformer(client, namespace, 5*time.Minute, nil, func(options *metav1.ListOptions) { options.FieldSelector = fmt.Sprintf("metadata.name=%s", name) })
	if logger == nil {
		logger = log.NewNopLogger()
	}
	c := controller{
		client:   client,
		path:     path,
		key:      key,
		informer: pi,
		logger:   logger,
		reconcileAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "reconcile_attempts_total",
			Help: "Number of attempts to reconcile the ConfigMap to disk",
		}),
		reconcileErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "reconcile_errors_total",
			Help: "Number of errors that occurred while reconciling the ConfigMap to disk",
		}),
	}

	if reg != nil {
		reg.MustRegister(c.reconcileAttempts, c.reconcileErrors)
	}

	return &c
}

func (c *controller) run(stop <-chan struct{}) error {
	go c.informer.Run(stop)
	if ok := cache.WaitForCacheSync(stop, func() bool {
		return c.informer.HasSynced()
	}); !ok {
		return errors.New("sync peer cache")
	}

	// Add handlers after initial refresh and sync.
	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handle,
		UpdateFunc: func(_, obj interface{}) { c.handle(obj) },
		DeleteFunc: func(_ interface{}) {
			c.reconcileAttempts.Inc()
			if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
				c.reconcileErrors.Inc()
				level.Error(c.logger).Log("err", fmt.Sprintf("failed to delete file: %v", err))
			}
		},
	})

	<-stop
	return nil
}

func (c *controller) handle(obj interface{}) {
	c.reconcileAttempts.Inc()
	cm, ok := obj.(*v1.ConfigMap)
	if !ok {
		level.Warn(c.logger).Log("msg", "object is not a ConfigMap")
		return
	}

	data, ok := cm.Data[c.key]
	if !ok {
		if err := os.Remove(c.path); err != nil {
			c.reconcileErrors.Inc()
			level.Error(c.logger).Log("err", fmt.Sprintf("failed to delete file: %v", err))
		}
		return
	}

	if err := ioutil.WriteFile(c.path, []byte(data), 0644); err != nil {
		c.reconcileErrors.Inc()
		level.Error(c.logger).Log("err", fmt.Sprintf("failed to write file: %v", err))
	}
	return
}

func runOneTime(client kubernetes.Interface, namespace, path, name, key string, logger log.Logger) error {
	// get configmap
	cm, err := client.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})

	// Raise err if configmap doesn't exist
	if err != nil {
		level.Error(logger).Log("err", fmt.Sprintf("Failed to retrieve ConfigMap: %v", err))
		return err
	}

	// safe to file.
	data, ok := cm.Data[key]
	if !ok {
		level.Error(logger).Log("err", fmt.Sprintf("Configmap does not have key: %v", key))
		return fmt.Errorf("Configmap does not include specified key: %v", key)
	}

	if err := ioutil.WriteFile(path, []byte(data), 0644); err != nil {
		level.Error(logger).Log("err", fmt.Sprintf("failed to write file: %v", err))
	}

	return nil
}
