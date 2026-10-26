#!/usr/bin/env python3
"""AutoWonder 模板批量初始化（原始 JSON-RPC，仅标准库）。

把实例化后的 harness 模板（见 references/onboarding.md §3）按平台要求的顺序写入
AutoWonder 工作空间，并做回读断言。所有环境信息来自命令行参数，本脚本不含任何
内置地址或令牌。

用法（按序执行，任一阶段失败即停）:
  bulk_init.py --url <mcp-url> --workspace-id <id> --instance inst-std.json --instance inst-fast.json sdlcs
  bulk_init.py ... steps      # 加步骤并启用 SDLC
  bulk_init.py ... agents     # 创建数字人
  bulk_init.py ... config     # 绑定 SDLC / evolutionMode
  bulk_init.py ... publish    # 提交审核并发布
  bulk_init.py ... squads     # 建小队 + 加成员
  bulk_init.py ... repos --repo-id <id> --perm WRITE   # 绑仓库并重新发布
  bulk_init.py ... verify     # 回读逐项断言
  bulk_init.py ... all        # 除 repos 外按序全跑

阶段间状态（SDLC/数字人/小队 ID 映射）保存在 --state 指定文件（默认 ./aw-init-state.json）。
"""
import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from aw_rpc import McpClient


class State:
    def __init__(self, path):
        self.path = Path(path)
        self.data = json.loads(self.path.read_text(encoding="utf-8")) if self.path.exists() else {}

    def save(self):
        self.path.write_text(json.dumps(self.data, ensure_ascii=False, indent=2), encoding="utf-8")

    def sdlc_id(self, name):
        sid = self.data.get("sdlcs", {}).get(name)
        if not sid:
            raise SystemExit(f"状态里没有 SDLC「{name}」的 ID，先跑 sdlcs/steps 阶段")
        return sid

    def agent_id(self, role_code):
        aid = self.data.get("agents", {}).get(role_code)
        if not aid:
            raise SystemExit(f"状态里没有数字人「{role_code}」的 ID，先跑 agents 阶段")
        return aid


def load_instances(paths):
    out = []
    for p in paths:
        t = json.loads(Path(p).read_text(encoding="utf-8"))
        # 结构契约校验（失败即停，不写平台）
        assert t.get("template", {}).get("name") and t["template"].get("description"), f"{p}: template 缺字段"
        assert t.get("squad", {}).get("name") and t.get("agents"), f"{p}: squad/agents 缺失"
        assert t["template"]["squadSize"] == len(t["agents"]), f"{p}: squadSize 与 agents 数不符"
        names = [a["name"] for a in t["agents"]]
        codes = [a["roleCode"] for a in t["agents"]]
        assert len(set(names)) == len(names) and len(set(codes)) == len(codes), f"{p}: name/roleCode 不唯一"
        for a in t["agents"]:
            sdlc = a.get("sdlc")
            if sdlc:
                assert sdlc.get("name") and sdlc.get("steps"), f"{p}: {a['name']} SDLC 缺 name/steps"
                orders = [s["order"] for s in sdlc["steps"]]
                assert orders == sorted(orders) and len(set(orders)) == len(orders), f"{p}: order 非严格升序"
                assert sdlc.get("isDefault") is not True, f"{p}: isDefault=true 不支持"
        out.append(t)
    # 跨模板唯一性：phase_agents/config 按 roleCode 全局索引，跨模板冲突会静默漏建数字人
    # 并把错误 SDLC 绑到共享数字人上。冲突时在实例副本里给冲突方加前缀改写（如 AWSELF_）
    all_names = [a["name"] for t in out for a in t["agents"]]
    all_codes = [a["roleCode"] for t in out for a in t["agents"]]
    assert len(set(all_names)) == len(all_names), \
        f"跨模板数字人 name 冲突: {[n for n in set(all_names) if all_names.count(n) > 1]}"
    assert len(set(all_codes)) == len(all_codes), \
        f"跨模板 roleCode 冲突: {[c for c in set(all_codes) if all_codes.count(c) > 1]}（改写冲突方加前缀，如 AWSELF_；两模板 handlerRoleRef 无跨引用时安全）"
    return out


def phase_sdlcs(cli, ws, instances, st):
    st.data.setdefault("sdlcs", {})
    for t in instances:
        for a in t["agents"]:
            sdlc = a.get("sdlc")
            if not sdlc or sdlc["name"] in st.data["sdlcs"]:
                continue
            args = {"workspaceId": ws, "name": sdlc["name"], "description": sdlc["description"]}
            if sdlc.get("workType"):
                args["workType"] = sdlc["workType"]
            r = cli.tool("autowonder.create_sdlc", **args)
            st.data["sdlcs"][sdlc["name"]] = r["id"]
            print(f"sdlc ok: {sdlc['name']} -> {r['id']}")
    st.save()


def phase_steps(cli, ws, instances, st):
    for t in instances:
        for a in t["agents"]:
            sdlc_cfg = a.get("sdlc")
            if not sdlc_cfg:
                continue
            sid = st.sdlc_id(sdlc_cfg["name"])
            for s in sdlc_cfg["steps"]:
                args = {
                    "workspaceId": ws, "sdlcId": sid,
                    "stepOrder": s["order"], "name": s["name"], "kind": s["kind"],
                    "instructionMd": s["instruction"],
                    "checklistJson": json.dumps(s["checklist"], ensure_ascii=False) if s.get("checklist") is not None else None,
                    "gatePolicyJson": json.dumps(s["gatePolicy"], ensure_ascii=False) if s.get("gatePolicy") is not None else None,
                    "required": bool(s.get("required")),
                    "retryBudget": s.get("retryBudget"),
                    "code": s.get("code"),
                    "handlerType": s.get("handlerType"), "handlerRoleRef": s.get("handlerRoleRef"),
                    "onSuccess": json.dumps(s["onSuccess"], ensure_ascii=False) if s.get("onSuccess") is not None else None,
                    "onFail": json.dumps(s["onFail"], ensure_ascii=False) if s.get("onFail") is not None else None,
                }
                if s.get("timeoutSeconds") is not None:
                    args["timeoutSeconds"] = s["timeoutSeconds"]
                if s.get("statusOnEnterCode") is not None:
                    args["statusOnEnterCode"] = s["statusOnEnterCode"]
                r = cli.tool("autowonder.add_sdlc_step", **args)
                print(f"step ok: sdlc={sid} order={s['order']} {s['name']} -> {r.get('id')}")
    for name, sid in st.data.get("sdlcs", {}).items():
        cli.tool("autowonder.enable_sdlc", workspaceId=ws, id=sid)
        print(f"enabled: {name}({sid})")


def phase_agents(cli, ws, instances, st):
    st.data.setdefault("agents", {})
    for t in instances:
        for a in t["agents"]:
            if a["roleCode"] in st.data["agents"]:
                continue
            r = cli.tool("autowonder.create_agent", workspaceId=ws, name=a["name"],
                         roleCode=a["roleCode"], roleName=a["roleName"],
                         soulMd=a["businessBackground"], agentMd=a["responsibilities"])
            st.data["agents"][a["roleCode"]] = r["id"]
            print(f"agent ok: {a['name']} ({a['roleCode']}) -> {r['id']}")
    st.save()


def phase_config(cli, ws, instances, st):
    for t in instances:
        for a in t["agents"]:
            sdlc = a.get("sdlc")
            cli.tool("autowonder.update_agent_config", workspaceId=ws,
                     agentId=st.agent_id(a["roleCode"]),
                     sdlcId=st.sdlc_id(sdlc["name"]) if sdlc else None,
                     evolutionMode=a.get("evolutionMode"))
            print(f"config ok: {a['name']} sdlcId={sdlc['name'] if sdlc else None}")


def phase_publish(cli, ws, instances, st):
    for t in instances:
        for a in t["agents"]:
            aid = st.agent_id(a["roleCode"])
            cli.tool("autowonder.submit_agent_for_review", workspaceId=ws, id=aid)
            cli.tool("autowonder.publish_agent", workspaceId=ws, id=aid)
            print(f"published: {a['name']} ({aid})")


def phase_squads(cli, ws, instances, st):
    st.data.setdefault("squads", {})
    for t in instances:
        sq = t["squad"]
        if sq["name"] in st.data["squads"]:
            sqid = st.data["squads"][sq["name"]]
        else:
            r = cli.tool("autowonder.create_squad", workspaceId=ws,
                         name=sq["name"], description=sq.get("description"))
            sqid = r["id"]
            st.data["squads"][sq["name"]] = sqid
            st.save()
        print(f"squad ok: {sq['name']} -> {sqid}")
        for a in t["agents"]:
            cli.tool("autowonder.add_agent_to_squad", workspaceId=ws,
                     squadId=sqid, agentId=st.agent_id(a["roleCode"]))
            print(f"  member+: {a['name']}")


def phase_repos(cli, ws, instances, st, repo_id, perm):
    ids = [st.agent_id(a["roleCode"]) for t in instances for a in t["agents"]]
    for aid in ids:
        cli.tool("autowonder.bind_agent_repos", workspaceId=ws, agentId=aid,
                 repoIds=[repo_id], permLevel=perm)
    # 权限绑定写入编辑版本，必须重新提交+发布才进在线版本
    phase_publish(cli, ws, instances, st)
    print(f"repo {repo_id} bound to {len(ids)} agents ({perm}) and republished")


def phase_verify(cli, ws, instances, st):
    fails = []
    for t in instances:
        for a in t["agents"]:
            sdlc_cfg = a.get("sdlc")
            if not sdlc_cfg:
                continue
            got = cli.tool("autowonder.get_sdlc", workspaceId=ws, id=st.sdlc_id(sdlc_cfg["name"]))
            steps = sorted(got.get("steps") or [], key=lambda s: s["stepOrder"])
            if got.get("status") != "ENABLED":
                fails.append(f"SDLC {sdlc_cfg['name']} status={got.get('status')}")
            if len(steps) != len(sdlc_cfg["steps"]):
                fails.append(f"SDLC {sdlc_cfg['name']} 步骤数 {len(steps)} != {len(sdlc_cfg['steps'])}")
                continue
            for cfg, g in zip(sdlc_cfg["steps"], steps):
                for f_cfg, f_got in [("name", "name"), ("kind", "kind"), ("code", "code"),
                                      ("instruction", "instructionMd"), ("handlerType", "handlerType"),
                                      ("handlerRoleRef", "handlerRoleRef")]:
                    if cfg.get(f_cfg) != g.get(f_got):
                        fails.append(f"{sdlc_cfg['name']}#{cfg['order']} {f_cfg} 不一致")
            print(f"SDLC: {sdlc_cfg['name']} steps={len(steps)} status={got.get('status')}")
    for t in instances:
        for a in t["agents"]:
            aid = st.agent_id(a["roleCode"])
            ag = cli.tool("autowonder.get_agent", workspaceId=ws, id=aid)
            if ag.get("status") != "ONLINE":
                fails.append(f"{a['name']} status={ag.get('status')}")
            vno = ag.get("latestVersionNo") or 1
            v = cli.tool("autowonder.get_agent_version", workspaceId=ws, agentId=aid, versionNo=vno)
            want_sdlc = st.sdlc_id(a["sdlc"]["name"]) if a.get("sdlc") else None
            if v.get("sdlcId") != want_sdlc:
                fails.append(f"{a['name']} sdlcId={v.get('sdlcId')} 期望 {want_sdlc}")
            if v.get("businessBackground") != a["businessBackground"]:
                fails.append(f"{a['name']} soulMd 不一致（疑似截断）")
            if v.get("responsibilities") != a["responsibilities"]:
                fails.append(f"{a['name']} agentMd 不一致（疑似截断）")
            print(f"agent: {a['name']} status={ag.get('status')} sdlcId={v.get('sdlcId')} 内容一致")
    for t in instances:
        sqid = st.data.get("squads", {}).get(t["squad"]["name"])
        if not sqid:
            fails.append(f"小队 {t['squad']['name']} 未创建")
            continue
        g = cli.tool("autowonder.get_squad", workspaceId=ws, id=sqid)
        want = sorted(st.agent_id(a["roleCode"]) for a in t["agents"])
        got = sorted(g.get("memberAgentIds") or [])
        if got != want:
            fails.append(f"小队 {t['squad']['name']} 成员不一致: {got} != {want}")
        else:
            print(f"squad: {t['squad']['name']} 成员 {len(want)} 一致")
    print()
    if fails:
        print("!!! 校验失败:")
        for f in fails:
            print(" -", f)
        raise SystemExit(1)
    print("=== 回读校验全部通过 ===")


PHASES = {k[len("phase_"):]: v for k, v in list(globals().items())
          if k.startswith("phase_") and callable(v)}


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--url", required=True, help="AutoWonder MCP 端点")
    ap.add_argument("--workspace-id", type=int, required=True)
    ap.add_argument("--instance", action="append", required=True,
                    help="实例化模板 JSON 路径（可多次）")
    ap.add_argument("--state", default="aw-init-state.json", help="ID 状态文件")
    ap.add_argument("--repo-id", type=int, help="repos 阶段：要绑定的仓库 ID")
    ap.add_argument("--perm", default="WRITE", choices=["READ", "WRITE", "ADMIN"])
    ap.add_argument("phase", choices=list(PHASES) + ["all"])
    ns = ap.parse_args()

    cli = McpClient(ns.url)
    cli.initialize()
    instances = load_instances(ns.instance)
    st = State(ns.state)

    if ns.phase == "all":
        for name in ["sdlcs", "steps", "agents", "config", "publish", "squads", "verify"]:
            print(f"---- phase {name}")
            PHASES[name](cli, ns.workspace_id, instances, st)
    elif ns.phase == "repos":
        if not ns.repo_id:
            raise SystemExit("repos 阶段需要 --repo-id")
        phase_repos(cli, ns.workspace_id, instances, st, ns.repo_id, ns.perm)
    else:
        PHASES[ns.phase](cli, ns.workspace_id, instances, st)
    print(f"== phase {ns.phase} DONE ==")


if __name__ == "__main__":
    main()
