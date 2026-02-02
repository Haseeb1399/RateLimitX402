package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockLimiter is a mock implementation of ratelimit.Limiter
type MockLimiter struct {
	mock.Mock
}

func (m *MockLimiter) Allow(key string) (bool, error) {
	args := m.Called(key)
	return args.Bool(0), args.Error(1)
}

func (m *MockLimiter) Refill(key string, tokens float64) error {
	args := m.Called(key, tokens)
	return args.Error(0)
}

func (m *MockLimiter) Available(key string) (float64, error) {
	args := m.Called(key)
	return args.Get(0).(float64), args.Error(1)
}

func TestRateLimitMiddleware_Allowed(t *testing.T) {
	// Setup
	limiter := new(MockLimiter)
	limiter.On("Allow", mock.Anything).Return(true, nil)

	handler := RateLimitMiddleware(limiter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	limiter.AssertExpectations(t)
}

func TestRateLimitMiddleware_RateLimited(t *testing.T) {
	// Setup
	limiter := new(MockLimiter)
	limiter.On("Allow", mock.Anything).Return(false, nil)

	handler := RateLimitMiddleware(limiter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "1", w.Header().Get("Retry-After"))
	limiter.AssertExpectations(t)
}

func TestGinRateLimitMiddleware_Allowed(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	limiter := new(MockLimiter)
	limiter.On("Allow", mock.Anything).Return(true, nil)

	r := gin.New()
	r.Use(GinRateLimitMiddleware(limiter))
	r.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Execute
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	limiter.AssertExpectations(t)
}

func TestGinRateLimitMiddleware_RateLimited(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	limiter := new(MockLimiter)
	limiter.On("Allow", mock.Anything).Return(false, nil)

	r := gin.New()
	r.Use(GinRateLimitMiddleware(limiter))
	r.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Execute
	r.ServeHTTP(w, req)

	// Assert
	// GinRateLimitMiddleware returns 402 Payment Required
	assert.Equal(t, http.StatusPaymentRequired, w.Code)
	limiter.AssertExpectations(t) // Ensure Allow was called
}
