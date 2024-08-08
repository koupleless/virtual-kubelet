package prometheus

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

func TestStartPrometheusListen(t *testing.T) {
	go StartPrometheusListen(10080)
	_, err := http.Get("http://localhost:10080/metrics")
	assert.Nil(t, err)
}
