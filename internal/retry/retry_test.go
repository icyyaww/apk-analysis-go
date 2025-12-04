package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestRetry_Success 测试第一次就成功的情况
func TestRetry_Success(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := Retry(ctx, func(ctx context.Context) error {
		attempts++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, attempts, "Should succeed on first attempt")
}

// TestRetry_SuccessAfterRetries 测试重试后成功
func TestRetry_SuccessAfterRetries(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := RetryWithAttempts(ctx, 5, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, attempts, "Should succeed on third attempt")
}

// TestRetry_MaxAttemptsReached 测试达到最大尝试次数
func TestRetry_MaxAttemptsReached(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	maxAttempts := 3

	err := RetryWithAttempts(ctx, maxAttempts, func(ctx context.Context) error {
		attempts++
		return errors.New("persistent error")
	})

	assert.Error(t, err)
	assert.Equal(t, maxAttempts, attempts, "Should attempt exactly max times")
	assert.Contains(t, err.Error(), "max attempts")
}

// TestRetry_ContextCanceled 测试上下文取消
func TestRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := RetryWithAttempts(ctx, 10, func(ctx context.Context) error {
		attempts++
		time.Sleep(200 * time.Millisecond)
		return errors.New("slow operation")
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "canceled")
	assert.Less(t, attempts, 10, "Should stop before max attempts")
}

// TestRetry_Timeout 测试超时
func TestRetry_Timeout(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.Timeout = 500 * time.Millisecond
	config.MaxAttempts = 10
	config.InitialInterval = 200 * time.Millisecond
	attempts := 0

	err := Do(ctx, config, func(ctx context.Context) error {
		attempts++
		time.Sleep(100 * time.Millisecond)
		return errors.New("slow operation")
	})

	assert.Error(t, err)
	assert.Less(t, attempts, 10, "Should stop due to timeout")
}

// TestRetry_NonRetryableError 测试不可重试错误
func TestRetry_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := RetryWithAttempts(ctx, 5, func(ctx context.Context) error {
		attempts++
		return NewNonRetryableError(errors.New("fatal error"))
	})

	assert.Error(t, err)
	assert.Equal(t, 1, attempts, "Should not retry non-retryable error")
	assert.Contains(t, err.Error(), "non-retryable")
}

// TestRetry_RetryableError 测试可重试错误
func TestRetry_RetryableError(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	maxAttempts := 3

	err := RetryWithAttempts(ctx, maxAttempts, func(ctx context.Context) error {
		attempts++
		return NewRetryableError(errors.New("temporary error"))
	})

	assert.Error(t, err)
	assert.Equal(t, maxAttempts, attempts, "Should retry all attempts")
}

// TestRetry_FixedStrategy 测试固定间隔策略
func TestRetry_FixedStrategy(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.Strategy = StrategyFixed
	config.InitialInterval = 100 * time.Millisecond
	config.MaxAttempts = 3
	config.Logger = logrus.New()
	config.Logger.SetLevel(logrus.ErrorLevel)

	attempts := 0
	startTime := time.Now()

	err := Do(ctx, config, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	duration := time.Since(startTime)

	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)
	// 应该等待 2 次（第1次失败后等待，第2次失败后等待）
	// 每次等待 100ms，总共约 200ms（加上执行时间）
	assert.GreaterOrEqual(t, duration, 200*time.Millisecond)
	assert.Less(t, duration, 400*time.Millisecond) // 给一些缓冲
}

// TestRetry_LinearStrategy 测试线性递增策略
func TestRetry_LinearStrategy(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.Strategy = StrategyLinear
	config.InitialInterval = 100 * time.Millisecond
	config.MaxAttempts = 4
	config.Logger = logrus.New()
	config.Logger.SetLevel(logrus.ErrorLevel)

	attempts := 0
	startTime := time.Now()

	err := Do(ctx, config, func(ctx context.Context) error {
		attempts++
		if attempts < 4 {
			return errors.New("temporary error")
		}
		return nil
	})

	duration := time.Since(startTime)

	assert.NoError(t, err)
	assert.Equal(t, 4, attempts)
	// 线性递增: 100ms (attempt 1), 200ms (attempt 2), 300ms (attempt 3)
	// 总共约 600ms
	assert.GreaterOrEqual(t, duration, 600*time.Millisecond)
	assert.Less(t, duration, 900*time.Millisecond)
}

// TestRetry_ExponentialStrategy 测试指数退避策略
func TestRetry_ExponentialStrategy(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.Strategy = StrategyExponential
	config.InitialInterval = 100 * time.Millisecond
	config.MaxAttempts = 4
	config.Logger = logrus.New()
	config.Logger.SetLevel(logrus.ErrorLevel)

	attempts := 0
	startTime := time.Now()

	err := Do(ctx, config, func(ctx context.Context) error {
		attempts++
		if attempts < 4 {
			return errors.New("temporary error")
		}
		return nil
	})

	duration := time.Since(startTime)

	assert.NoError(t, err)
	assert.Equal(t, 4, attempts)
	// 指数退避: 100ms (2^0), 200ms (2^1), 400ms (2^2)
	// 总共约 700ms
	assert.GreaterOrEqual(t, duration, 700*time.Millisecond)
	assert.Less(t, duration, 1000*time.Millisecond)
}

// TestRetry_MaxInterval 测试最大间隔限制
func TestRetry_MaxInterval(t *testing.T) {
	config := &Config{
		MaxAttempts:     5,
		InitialInterval: 1 * time.Second,
		MaxInterval:     2 * time.Second,
		Strategy:        StrategyExponential,
		Logger:          logrus.New(),
	}
	config.Logger.SetLevel(logrus.ErrorLevel)

	// 计算间隔
	interval1 := calculateNextInterval(config.Strategy, config.InitialInterval, config.InitialInterval, config.MaxInterval, 1)
	interval2 := calculateNextInterval(config.Strategy, interval1, config.InitialInterval, config.MaxInterval, 2)
	interval3 := calculateNextInterval(config.Strategy, interval2, config.InitialInterval, config.MaxInterval, 3)

	// 指数退避: 1s, 2s, 4s（被限制为2s）
	assert.Equal(t, 1*time.Second, interval1)
	assert.Equal(t, 2*time.Second, interval2)
	assert.Equal(t, 2*time.Second, interval3) // 被最大间隔限制
}

// TestDoWithResult_Success 测试带返回值的重试（成功）
func TestDoWithResult_Success(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.Logger.SetLevel(logrus.ErrorLevel)
	attempts := 0

	result, err := DoWithResult(ctx, config, func(ctx context.Context) (string, error) {
		attempts++
		if attempts < 2 {
			return "", errors.New("temporary error")
		}
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 2, attempts)
}

// TestDoWithResult_Failure 测试带返回值的重试（失败）
func TestDoWithResult_Failure(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.MaxAttempts = 2
	config.Logger.SetLevel(logrus.ErrorLevel)
	attempts := 0

	result, err := DoWithResult(ctx, config, func(ctx context.Context) (string, error) {
		attempts++
		return "", errors.New("persistent error")
	})

	assert.Error(t, err)
	assert.Equal(t, "", result)
	assert.Equal(t, 2, attempts)
}

// TestWithRetry_HigherOrderFunction 测试高阶函数
func TestWithRetry_HigherOrderFunction(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.MaxAttempts = 3
	config.Logger.SetLevel(logrus.ErrorLevel)

	attempts := 0
	originalFunc := func(ctx context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	}

	// 包装函数
	retryableFunc := WithRetry(config, originalFunc)

	// 执行
	err := retryableFunc(ctx)

	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

// TestIsRetryable_DefaultBehavior 测试默认重试行为
func TestIsRetryable_DefaultBehavior(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		retryable  bool
	}{
		{
			name:      "Nil error",
			err:       nil,
			retryable: false,
		},
		{
			name:      "Context canceled",
			err:       context.Canceled,
			retryable: false,
		},
		{
			name:      "Context deadline exceeded",
			err:       context.DeadlineExceeded,
			retryable: false,
		},
		{
			name:      "Generic error",
			err:       errors.New("some error"),
			retryable: true,
		},
		{
			name:      "Wrapped retryable error",
			err:       NewRetryableError(errors.New("retryable")),
			retryable: true,
		},
		{
			name:      "Wrapped non-retryable error",
			err:       NewNonRetryableError(errors.New("fatal")),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			assert.Equal(t, tt.retryable, result)
		})
	}
}

// BenchmarkRetry_Success 基准测试：成功情况
func BenchmarkRetry_Success(b *testing.B) {
	ctx := context.Background()
	config := DefaultConfig()
	config.Logger.SetLevel(logrus.ErrorLevel)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Do(ctx, config, func(ctx context.Context) error {
			return nil
		})
	}
}

// BenchmarkRetry_OneRetry 基准测试：一次重试
func BenchmarkRetry_OneRetry(b *testing.B) {
	ctx := context.Background()
	config := DefaultConfig()
	config.InitialInterval = 1 * time.Millisecond
	config.Logger.SetLevel(logrus.ErrorLevel)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		attempts := 0
		Do(ctx, config, func(ctx context.Context) error {
			attempts++
			if attempts < 2 {
				return errors.New("temp error")
			}
			return nil
		})
	}
}

// BenchmarkCalculateNextInterval 基准测试：间隔计算
func BenchmarkCalculateNextInterval(b *testing.B) {
	initial := 1 * time.Second
	max := 30 * time.Second

	b.Run("Fixed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			calculateNextInterval(StrategyFixed, initial, initial, max, i)
		}
	})

	b.Run("Linear", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			calculateNextInterval(StrategyLinear, initial, initial, max, i)
		}
	})

	b.Run("Exponential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			calculateNextInterval(StrategyExponential, initial, initial, max, i)
		}
	})
}
