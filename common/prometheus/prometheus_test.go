package prometheus

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
	"time"
)

func TestStartPrometheusListen(t *testing.T) {
	go StartPrometheusListen(10080)
	time.Sleep(1 * time.Second)
	_, err := http.Get("http://localhost:10080/metrics")
	assert.Nil(t, err)
}
