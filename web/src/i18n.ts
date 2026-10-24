// i18n.ts：中文文案包（design.md D5：i18n 框架预留，v1 全中文；文案
// 集中在此，后续多语言时按 key 拆包）。
export const zh = {
  app: {
    name: 'awem',
    topbar: {
      daemon: 'daemon',
      platform: '平台',
      settings: '设置',
    },
  },
  nav: {
    overview: '总览',
    detail: '执行器详情',
    addImport: '添加 / 导入',
    settings: '设置',
  },
  login: {
    title: '连接 awem',
    hint: '输入本机 API token（data/api-token 文件内容）',
    placeholder: 'api-token',
    submit: '连接',
    wrong: 'token 不正确，请重新输入（可查看 <配置目录>/data/api-token）',
  },
  common: {
    loading: '加载中…',
    empty: '暂无数据',
    save: '保存',
    cancel: '取消',
    confirm: '确认',
    retry: '重试',
    errorTitle: '出错了',
    networkError: '无法连接 daemon：请确认 awem daemon 正在运行（任一 CLI 命令会自动拉起）',
  },
  status: {
    local: '本地进程',
    platform: '平台',
    running: '运行中',
    stopped: '已停止',
    backoff: '重连中',
    stopping: '停止中',
    failed: '失败',
    connected: '已连接',
    reconnecting: '重连中',
    unknown: '未知',
    unreachable: '不可达',
    unowned: '未归属',
    drift: '漂移',
  },
  actions: {
    start: '启动',
    stop: '停止',
    restart: '重启',
    remove: '删除',
    addExecutor: '添加执行器',
  },
  batch: {
    barLabel: '批量操作',
    selectAll: '全选执行器',
    selectRow: (name: string) => `选择执行器 ${name}`,
    selected: (n: number) => `已选 ${n} 项`,
    clear: '取消选择',
    action: {
      start: '启动',
      stop: '停止',
      delete: '删除',
    },
    stopDialogTitle: (n: number) => `停止 ${n} 个执行器`,
    stopDialogBody: (n: number) => `将终止 ${n} 个执行器的子进程（整棵进程树优雅终止）。`,
    stopDialogConfirm: (n: number) => `停止 ${n} 个执行器`,
    deleteDialogTitle: (n: number) => `删除 ${n} 个执行器`,
    deleteDialogIntro: (n: number) => `将删除选中的 ${n} 个执行器，此操作不可恢复。本地日志保留。`,
    deleteDialogConfirm: (n: number) => `删除 ${n} 个执行器`,
    deleteBlocked: (n: number, names: string) =>
      `有 ${n} 个运行中，请先停止后再删除${names ? `：${names}` : ''}`,
    done: {
      start: (n: number) => `已启动 ${n} 个执行器`,
      stop: (n: number) => `已停止 ${n} 个执行器`,
      delete: (n: number) => `已删除 ${n} 个执行器`,
    },
    partial: (action: string, ok: number, fail: number, detail: string) =>
      `已${action} ${ok} 个执行器，${fail} 个失败${detail ? `：${detail}` : ''}`,
    refreshFailed: '无法获取执行器最新状态，请重试',
    reason: {
      notFound: '不存在',
      running: '运行中冲突',
      unknown: '未知错误',
    },
  },
} as const

export type Texts = typeof zh
export const t = zh
