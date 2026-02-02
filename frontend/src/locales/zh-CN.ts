export default {
  app: {
    title: 'WillChat',
    theme: '主题',
  },
  winsnap: {
    title: 'WillChat',
    assistantName: '灵魂厨房助手',
    cancelSnap: '取消吸附',
    placeholder: '请输入问题',
  },
  nav: {
    assistant: 'AI助手',
    knowledge: '知识库',
    multiask: '多问',
    settings: '设置',
  },
  tabs: {
    newTab: '新标签页',
  },
  hello: {
    inputPlaceholder: '请在下方输入你的名字 👇',
    greetButton: '打招呼',
    defaultName: '匿名',
    showSettings: '显示设置',
    hideSettings: '隐藏设置',
    learnMore: '点击 Wails 图标了解更多',
    listeningEvent: '正在监听 Time 事件...',
  },
  settings: {
    title: '设置',
    refreshWindows: '刷新窗口列表',
    hideSettings: '隐藏设置窗口',
    windowStatus: '窗口状态',
    // 设置菜单
    menu: {
      modelService: '模型服务',
      generalSettings: '常规设置',
      snapSettings: '吸附设置',
      tools: '功能工具',
      about: '关于我们',
    },
    // 常规设置
    general: {
      title: '常规设置',
      language: '语言',
      theme: '主题颜色',
    },
    // 吸附设置
    snap: {
      title: '设置',
      showAiSendButton: 'AI回复显示发送到聊天按钮',
      sendKeyStrategy: '发送消息按键模式',
      showAiEditButton: 'AI回复显示编辑内容按钮',
      appsTitle: '吸附应用',
      sendKeyOptions: {
        enter: '按 Enter 键发送',
        ctrlEnter: '按 Ctrl+Enter 键发送',
      },
      apps: {
        wechat: '微信',
        wecom: '企业微信',
        qq: 'QQ',
        dingtalk: '钉钉',
        feishu: '飞书',
        douyin: '抖音',
      },
    },
    // 功能工具设置
    tools: {
      tray: {
        title: '托盘',
        showIcon: '显示托盘图标',
        minimizeOnClose: '关闭时最小化到托盘',
      },
      floatingWindow: {
        title: '悬浮窗',
        show: '显示悬浮窗',
      },
      selectionSearch: {
        title: '划词搜索',
        enable: '划词搜索',
      },
    },
    // 语言选项
    languages: {
      zhCN: '中文',
      enUS: 'English',
    },
    // 主题选项
    themes: {
      light: '浅色',
      dark: '深色',
      system: '跟随系统',
    },
    // 模型服务
    modelService: {
      enabled: '已启用',
      apiKey: 'API 密钥',
      apiKeyPlaceholder: '请输入 API 密钥',
      apiKeyRequired: '请先填写 API 密钥',
      apiEndpoint: 'API 地址',
      apiEndpointPlaceholder: '请输入 API 地址',
      apiEndpointHint: '可选，留空使用默认地址',
      apiEndpointRequired: '请先填写 API 地址',
      apiVersion: 'API 版本',
      apiVersionPlaceholder: '例如：2024-02-01',
      apiVersionRequired: '请先填写 API 版本',
      check: '检测',
      checkSuccess: '检测成功',
      checkFailed: '检测失败',
      reset: '重置',
      llmModels: '大语言模型',
      embeddingModels: '嵌入模型',
      noModels: '暂无模型',
      loadingProviders: '加载中...',
      loadFailed: '加载失败',
      formIncomplete: '请先完成必填项',
      // 模型增删改
      addModel: '添加模型',
      editModel: '编辑模型',
      deleteModel: '删除模型',
      modelId: '模型 ID',
      modelIdPlaceholder: '请输入模型 ID，如：gpt-4o',
      modelName: '模型名称',
      modelNamePlaceholder: '请输入模型显示名称',
      modelType: '模型类型',
      cancel: '取消',
      save: '保存',
      modelCreated: '模型添加成功',
      modelUpdated: '模型更新成功',
      modelDeleted: '模型删除成功',
      deleteConfirmTitle: '确认删除',
      deleteConfirmMessage: '确定要删除模型「{name}」吗？此操作无法撤销。',
      confirmDelete: '删除',
      deleting: '删除中...',
    },
    // 关于我们
    about: {
      title: '关于我们',
      appName: 'WillChat',
      copyright: '© 2026 武汉芝麻小客服网络科技有限公司 版权所有',
      officialWebsite: '官方网站',
      view: '查看',
    },
  },
}
