package retry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// Strategy 重试策略
type Strategy string

const (
	StrategyFixed       Strategy = "fixed"       // 固定间隔
	StrategyLinear      Strategy = "linear"      // 线性递增
	StrategyExponential Strategy = "exponential" // 指数退避
)

// Config 重试配置
type Config struct {
	MaxAttempts     int           // 最大尝试次数
	InitialInterval time.Duration // 初始间隔
	MaxInterval     time.Duration // 最大间隔
	Strategy        Strategy      // 重试策略
	Timeout         time.Duration // 总超时时间
	Logger          *logrus.Logger
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxAttempts:     3,
		InitialInterval: 1 * time.Second,
		MaxInterval:     30 * time.Second,
		Strategy:        StrategyExponential,
		Timeout:         5 * time.Minute,
		Logger:          logrus.New(),
	}
}

// RetryableError 可重试错误接口
type RetryableError interface {
	error
	IsRetryable() bool
}

// retryableError 实现可重试错误
type retryableError struct {
	error
	retryable bool
}

func (e *retryableError) IsRetryable() bool {
	return e.retryable
}

// NewRetryableError 创建可重试错误
func NewRetryableError(err error) error {
	return &retryableError{
		error:     err,
		retryable: true,
	}
}

// NewNonRetryableError 创建不可重试错误
func NewNonRetryableError(err error) error {
	return &retryableError{
		error:     err,
		retryable: false,
	}
}

// IsRetryable 判断错误是否可重试
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// 检查是否实现了 RetryableError 接口
	var retryableErr RetryableError
	if errors.As(err, &retryableErr) {
		return retryableErr.IsRetryable()
	}

	// 默认常见错误类型的重试策略
	switch {
	case errors.Is(err, context.Canceled):
		return false // 用户取消，不重试
	case errors.Is(err, context.DeadlineExceeded):
		return false // 超时，不重试
	default:
		return true // 默认可重试
	}
}

// Func 可重试的函数类型
type Func func(ctx context.Context) error

// Do 执行带重试的操作
func Do(ctx context.Context, config *Config, fn Func) error {
	if config == nil {
		config = DefaultConfig()
	}

	// 创建超时上下文
	var cancel context.CancelFunc
	if config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	var lastErr error
	interval := config.InitialInterval

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return fmt.Errorf("retry canceled: %w", ctx.Err())
		default:
		}

		// 执行函数
		startTime := time.Now()
		err := fn(ctx)
		duration := time.Since(startTime)

		// 成功
		if err == nil {
			if attempt > 1 {
				config.Logger.WithFields(logrus.Fields{
					"attempt":  attempt,
					"duration": duration,
				}).Info("Operation succeeded after retry")
			}
			return nil
		}

		lastErr = err

		// 记录失败
		config.Logger.WithFields(logrus.Fields{
			"attempt":  attempt,
			"max":      config.MaxAttempts,
			"duration": duration,
			"error":    err.Error(),
		}).Warn("Operation failed")

		// 检查是否可重试
		if !IsRetryable(err) {
			config.Logger.WithError(err).Warn("Error is not retryable, aborting")
			return fmt.Errorf("non-retryable error: %w", err)
		}

		// 最后一次尝试，不再等待
		if attempt >= config.MaxAttempts {
			break
		}

		// 计算下一次重试间隔
		interval = calculateNextInterval(config.Strategy, interval, config.InitialInterval, config.MaxInterval, attempt)

		// 等待重试
		config.Logger.WithFields(logrus.Fields{
			"next_attempt": attempt + 1,
			"wait":         interval,
		}).Info("Waiting before retry")

		select {
		case <-ctx.Done():
			return fmt.Errorf("retry canceled during wait: %w", ctx.Err())
		case <-time.After(interval):
			// 继续下一次尝试
		}
	}

	return fmt.Errorf("max attempts (%d) reached: %w", config.MaxAttempts, lastErr)
}

// calculateNextInterval 计算下一次重试间隔
func calculateNextInterval(strategy Strategy, current, initial, max time.Duration, attempt int) time.Duration {
	var next time.Duration

	switch strategy {
	case StrategyFixed:
		// 固定间隔
		next = initial

	case StrategyLinear:
		// 线性递增: initial * attempt
		next = initial * time.Duration(attempt)

	case StrategyExponential:
		// 指数退避: initial * 2^(attempt-1)
		multiplier := 1 << (attempt - 1) // 2^(attempt-1)
		next = initial * time.Duration(multiplier)

	default:
		next = initial
	}

	// 限制最大间隔
	if next > max {
		next = max
	}

	return next
}

// WithRetry 高阶函数：包装函数使其支持重试
func WithRetry(config *Config, fn Func) Func {
	return func(ctx context.Context) error {
		return Do(ctx, config, fn)
	}
}

// DoWithResult 执行带重试的操作（返回结果）
func DoWithResult[T any](ctx context.Context, config *Config, fn func(ctx context.Context) (T, error)) (T, error) {
	var result T
	var resultErr error

	err := Do(ctx, config, func(ctx context.Context) error {
		res, err := fn(ctx)
		if err != nil {
			return err
		}
		result = res
		return nil
	})

	if err != nil {
		resultErr = err
	}

	return result, resultErr
}

// Retry 简化版重试函数（使用默认配置）
func Retry(ctx context.Context, fn Func) error {
	return Do(ctx, DefaultConfig(), fn)
}

// RetryWithAttempts 指定尝试次数的重试
func RetryWithAttempts(ctx context.Context, attempts int, fn Func) error {
	config := DefaultConfig()
	config.MaxAttempts = attempts
	return Do(ctx, config, fn)
}

// RetryWithBackoff 指定退避策略的重试
func RetryWithBackoff(ctx context.Context, strategy Strategy, fn Func) error {
	config := DefaultConfig()
	config.Strategy = strategy
	return Do(ctx, config, fn)
}
