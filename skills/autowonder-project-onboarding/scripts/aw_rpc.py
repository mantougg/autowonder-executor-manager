#!/usr/bin/env python3
"""AutoWonder MCP 原始 JSON-RPC 客户端（仅标准库）。

宿主 MCP 客户端不可用、或读接口因 output schema 校验报 -32602 时，用本脚本
直连 AutoWonder MCP 的 Streamable HTTP 端点。注意原始通道的工具名是点号前缀
（autowonder.create_sdlc），可用 --list-tools 查看。

用法:
  aw_rpc.py --url http://<host>:<port>/api/mcp/<token>/ --list-tools
  aw_rpc.py --url <mcp-url> --call autowonder.list_sdlcs --args '{"workspaceId": 1}'
  aw_rpc.py --url <mcp-url> --call autowonder.add_sdlc_step --args-file req.json

大请求体一律走 --args-file（JSON 文件），避免 shell 内联截断含中文的长文本。
"""
import argparse
import json
import sys
import urllib.request


class McpClient:
    def __init__(self, url, timeout=120):
        self.url = url
        self.timeout = timeout
        self._id = 0
        self.session = None

    def _post(self, body, is_notification=False):
        req = urllib.request.Request(
            self.url,
            data=json.dumps(body).encode("utf-8"),
            headers={
                "Content-Type": "application/json",
                "Accept": "application/json, text/event-stream",
            },
        )
        if self.session:
            req.add_header("Mcp-Session-Id", self.session)
        with urllib.request.urlopen(req, timeout=self.timeout) as resp:
            sid = resp.headers.get("Mcp-Session-Id")
            if sid:
                self.session = sid
            raw = resp.read().decode("utf-8")
            ctype = resp.headers.get("Content-Type", "")
        if not raw.strip():
            return None  # 通知/已接受的空响应
        text = raw.strip()
        if ctype.startswith("text/event-stream"):
            for line in text.splitlines():
                if line.startswith("data:"):
                    text = line[5:].strip()
                    break
        return json.loads(text)

    def initialize(self):
        self._id += 1
        msg = self._post({
            "jsonrpc": "2.0", "id": self._id, "method": "initialize",
            "params": {
                "protocolVersion": "2025-03-26",
                "capabilities": {},
                "clientInfo": {"name": "aw-rpc", "version": "1.0"},
            },
        })
        if isinstance(msg, dict) and "error" in msg:
            raise RuntimeError(f"initialize 失败: {msg['error']}")
        self._post({"jsonrpc": "2.0", "method": "notifications/initialized"})
        return msg

    def call(self, method, params=None):
        self._id += 1
        body = {"jsonrpc": "2.0", "id": self._id, "method": method}
        if params is not None:
            body["params"] = params
        msg = self._post(body)
        if isinstance(msg, dict) and "error" in msg:
            raise RuntimeError(f"{method}: {msg['error']}")
        return msg.get("result") if isinstance(msg, dict) else msg

    def tool(self, tool_name, **arguments):
        # 参数名刻意叫 tool_name：AutoWonder 工具参数里常见 name=（如 create_sdlc），
        # 首参若也叫 name 会与之撞名报 TypeError
        r = self.call("tools/call", {"name": tool_name, "arguments": arguments})
        if r.get("isError"):
            txt = "".join(c.get("text", "") for c in r.get("content", []))
            raise RuntimeError(f"{tool_name}: {txt}")
        sc = r.get("structuredContent")
        if sc is not None:
            return sc
        txt = "".join(c.get("text", "") for c in r.get("content", []))
        try:
            return json.loads(txt)
        except (json.JSONDecodeError, TypeError):
            return txt

    def list_tools(self):
        r = self.call("tools/list")
        return [t["name"] for t in r.get("tools", [])]


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--url", required=True, help="AutoWonder MCP 端点（以 / 结尾）")
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument("--list-tools", action="store_true", help="列出全部工具名")
    g.add_argument("--call", metavar="TOOL", help="工具名，如 autowonder.list_sdlcs")
    ap.add_argument("--args", help='工具参数 JSON 字符串（短参数用）')
    ap.add_argument("--args-file", help="工具参数 JSON 文件（长参数/含中文必须用这个）")
    ns = ap.parse_args()

    cli = McpClient(ns.url)
    cli.initialize()
    if ns.list_tools:
        for n in cli.list_tools():
            print(n)
        return
    args = {}
    if ns.args_file:
        args = json.load(open(ns.args_file, encoding="utf-8"))
    elif ns.args:
        args = json.loads(ns.args)
    result = cli.tool(ns.call, **args)
    print(json.dumps(result, ensure_ascii=False, indent=2, default=str))


if __name__ == "__main__":
    sys.exit(main())
