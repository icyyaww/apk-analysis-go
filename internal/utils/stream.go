package utils

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
)

// StreamJSONLReader 流式 JSONL 读取器
type StreamJSONLReader struct {
	file    *os.File
	scanner *bufio.Scanner
	lineNum int
}

// NewStreamJSONLReader 创建流式 JSONL 读取器
func NewStreamJSONLReader(filePath string) (*StreamJSONLReader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	// 设置较大的缓冲区 (1MB) 以处理大行
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024) // 最大 10MB

	return &StreamJSONLReader{
		file:    file,
		scanner: scanner,
		lineNum: 0,
	}, nil
}

// ReadNext 读取下一行并解析为 map
func (r *StreamJSONLReader) ReadNext() (map[string]interface{}, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	r.lineNum++
	line := r.scanner.Bytes()

	var data map[string]interface{}
	if err := json.Unmarshal(line, &data); err != nil {
		return nil, err
	}

	return data, nil
}

// ReadNextTyped 读取下一行并解析为指定类型
func (r *StreamJSONLReader) ReadNextTyped(v interface{}) error {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return err
		}
		return io.EOF
	}

	r.lineNum++
	line := r.scanner.Bytes()

	return json.Unmarshal(line, v)
}

// LineNumber 获取当前行号
func (r *StreamJSONLReader) LineNumber() int {
	return r.lineNum
}

// Close 关闭读取器
func (r *StreamJSONLReader) Close() error {
	return r.file.Close()
}

// ReadJSONLFile 流式读取整个 JSONL 文件 (回调方式)
func ReadJSONLFile(filePath string, callback func(data map[string]interface{}) error) error {
	reader, err := NewStreamJSONLReader(filePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for {
		data, err := reader.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := callback(data); err != nil {
			return err
		}
	}

	return nil
}

// CountJSONLLines 统计 JSONL 文件行数 (不加载到内存)
func CountJSONLLines(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0

	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return count, nil
}

// StreamJSONLWriter 流式 JSONL 写入器
type StreamJSONLWriter struct {
	file   *os.File
	writer *bufio.Writer
}

// NewStreamJSONLWriter 创建流式 JSONL 写入器
func NewStreamJSONLWriter(filePath string) (*StreamJSONLWriter, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	writer := bufio.NewWriterSize(file, 64*1024) // 64KB 缓冲

	return &StreamJSONLWriter{
		file:   file,
		writer: writer,
	}, nil
}

// WriteLine 写入一行 JSON
func (w *StreamJSONLWriter) WriteLine(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if _, err := w.writer.Write(jsonData); err != nil {
		return err
	}

	if _, err := w.writer.WriteString("\n"); err != nil {
		return err
	}

	return nil
}

// Flush 刷新缓冲区
func (w *StreamJSONLWriter) Flush() error {
	return w.writer.Flush()
}

// Close 关闭写入器
func (w *StreamJSONLWriter) Close() error {
	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.file.Close()
}
