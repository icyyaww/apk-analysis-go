"""
Mitmproxy Addon: JSONL Flow Writer with Task-based File Isolation
实时将流量写入 JSONL 格式文件，每个任务独立文件，支持多设备并发

API Endpoints:
- POST /set_output    设置当前输出任务
  Body: {"task_id": "xxx"}
- POST /clear_output  清除当前输出（回到默认文件）
  Body: {}
- GET /current_task   获取当前任务ID
"""
import json
import time
from pathlib import Path
from mitmproxy import http, ctx
from threading import Lock
import threading
from http.server import HTTPServer, BaseHTTPRequestHandler


class JSONLFlowWriter:
    def __init__(self):
        # 默认输出文件（当没有活跃任务时）
        self.default_output_path = Path("/results/flows.jsonl")
        self.default_output_path.parent.mkdir(parents=True, exist_ok=True)

        # 当前活跃的任务ID和输出文件
        self.current_task_id = None
        self.current_file_path = None
        self.file = None
        self.lock = Lock()  # 线程安全

        # 打开默认文件
        self._open_file(str(self.default_output_path))

        print(f"[JSONL Writer] Started, default output: {self.default_output_path}")

    def _open_file(self, file_path):
        """
        打开文件用于写入
        """
        # 关闭旧文件
        if self.file is not None:
            try:
                self.file.close()
            except:
                pass

        # 确保目录存在
        Path(file_path).parent.mkdir(parents=True, exist_ok=True)

        # 打开新文件（追加模式）
        self.file = open(file_path, "a", buffering=1)  # Line buffering
        self.current_file_path = file_path

    def set_output_task(self, task_id):
        """
        设置当前输出任务（切换到任务专属文件）
        """
        with self.lock:
            self.current_task_id = task_id
            task_output_path = f"/results/{task_id}/flows.jsonl"
            self._open_file(task_output_path)
            print(f"[JSONL Writer] Output switched to task: {task_id} -> {task_output_path}")

    def clear_output_task(self):
        """
        清除当前任务（切换回默认文件）
        """
        with self.lock:
            old_task_id = self.current_task_id
            self.current_task_id = None
            self._open_file(str(self.default_output_path))
            print(f"[JSONL Writer] Output cleared (was task: {old_task_id}), back to default")

    def get_current_task(self):
        """
        获取当前任务ID
        """
        with self.lock:
            return {
                "task_id": self.current_task_id,
                "output_path": self.current_file_path,
            }

    def response(self, flow: http.HTTPFlow):
        """
        处理每个 HTTP 响应，将流量记录写入当前活跃任务的文件
        """
        try:
            with self.lock:
                current_task = self.current_task_id
                file_handle = self.file

            # 构建 FlowRecord 格式 (与 Go 代码中的 FlowRecord 结构匹配)
            record = {
                "ts": flow.request.timestamp_start,
                "method": flow.request.method,
                "scheme": flow.request.scheme,
                "host": flow.request.host,
                "port": flow.request.port,
                "path": flow.request.path,
                "url": flow.request.pretty_url,
            }

            # 如果有活跃任务，添加任务ID
            if current_task:
                record["task_id"] = current_task

            # 写入 JSONL
            json_line = json.dumps(record, ensure_ascii=False)
            file_handle.write(json_line + "\n")
            file_handle.flush()  # 立即刷新到磁盘

            # 日志（减少输出）
            if current_task:
                print(f"[JSONL Writer] [{current_task}] {record['method']} {record['url']}")

        except Exception as e:
            print(f"[JSONL Writer] Error: {e}")

    def done(self):
        """
        清理资源
        """
        with self.lock:
            if self.file is not None:
                try:
                    self.file.close()
                    print("[JSONL Writer] Closed file")
                except:
                    pass
                self.file = None


# 全局实例，用于 HTTP 服务器访问
_writer_instance = None


class TaskOutputHandler(BaseHTTPRequestHandler):
    """
    HTTP API Handler for task output management
    运行在独立端口 (8083) 供 Go 后端调用
    """

    def do_POST(self):
        try:
            content_length = int(self.headers.get('Content-Length', 0))
            body = self.rfile.read(content_length)
            data = json.loads(body.decode('utf-8')) if body else {}

            if self.path == "/set_output":
                task_id = data.get("task_id")
                if task_id:
                    _writer_instance.set_output_task(task_id)
                    self.send_response(200)
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(json.dumps({"status": "ok", "task_id": task_id}).encode('utf-8'))
                else:
                    self.send_response(400)
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(json.dumps({"error": "task_id required"}).encode('utf-8'))

            elif self.path == "/clear_output":
                _writer_instance.clear_output_task()
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({"status": "ok"}).encode('utf-8'))

            else:
                self.send_response(404)
                self.end_headers()

        except Exception as e:
            print(f"[API Error] {e}")
            self.send_response(500)
            self.end_headers()

    def do_GET(self):
        try:
            if self.path == "/current_task":
                task_info = _writer_instance.get_current_task()
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps(task_info).encode('utf-8'))
            else:
                self.send_response(404)
                self.end_headers()
        except Exception as e:
            print(f"[API Error] {e}")
            self.send_response(500)
            self.end_headers()

    def log_message(self, format, *args):
        # 减少日志输出
        pass


def start_http_server():
    """
    启动 HTTP 服务器用于任务输出管理 API
    """
    server = HTTPServer(('0.0.0.0', 8083), TaskOutputHandler)
    print("[Task Output API] Started on port 8083")
    server.serve_forever()


# Mitmproxy 会自动加载这个 addons 列表
_writer_instance = JSONLFlowWriter()
addons = [_writer_instance]

# 启动 HTTP API 服务器（在后台线程运行）
# 添加错误处理和日志
try:
    api_thread = threading.Thread(target=start_http_server, daemon=True)
    api_thread.start()
    print("[Task Output API] HTTP server thread started successfully")
except Exception as e:
    print(f"[Task Output API] ERROR: Failed to start HTTP server thread: {e}")
    import traceback
    traceback.print_exc()
