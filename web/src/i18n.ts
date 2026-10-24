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
} as const

export type Texts = typeof zh
export const t = zh
