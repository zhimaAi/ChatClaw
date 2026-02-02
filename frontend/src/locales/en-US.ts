export default {
  app: {
    title: 'WillChat',
    theme: 'Theme',
  },
  nav: {
    assistant: 'AI Assistant',
    knowledge: 'Knowledge Base',
    multiask: 'Multi Ask',
    settings: 'Settings',
  },
  tabs: {
    newTab: 'New Tab',
  },
  hello: {
    inputPlaceholder: 'Please enter your name below 👇',
    greetButton: 'Greet',
    defaultName: 'anonymous',
    showSettings: 'Show Settings',
    hideSettings: 'Hide Settings',
    learnMore: 'Click on the Wails logo to learn more',
    listeningEvent: 'Listening for Time event...',
  },
  settings: {
    title: 'Settings',
    refreshWindows: 'Refresh Window List',
    hideSettings: 'Hide Settings Window',
    windowStatus: 'Window Status',
    // Settings menu
    menu: {
      modelService: 'Model Service',
      generalSettings: 'General Settings',
      snapSettings: 'Snap Settings',
      tools: 'Tools',
      about: 'About Us',
    },
    // General settings
    general: {
      title: 'General Settings',
      language: 'Language',
      theme: 'Theme Color',
    },
    // Snap settings
    snap: {
      title: 'Settings',
      showAiSendButton: 'Show "Send to Chat" Button for AI Reply',
      sendKeyStrategy: 'Send Message Key Mode',
      showAiEditButton: 'Show "Edit Content" Button for AI Reply',
      appsTitle: 'Snap Apps',
      sendKeyOptions: {
        enter: 'Press Enter to send',
        ctrlEnter: 'Press Ctrl+Enter to send',
      },
      apps: {
        wechat: 'WeChat',
        wecom: 'WeCom',
        qq: 'QQ',
        dingtalk: 'DingTalk',
        feishu: 'Feishu',
        douyin: 'Douyin',
      },
    },
    // Tools settings
    tools: {
      tray: {
        title: 'System Tray',
        showIcon: 'Show Tray Icon',
        minimizeOnClose: 'Minimize to Tray on Close',
      },
      floatingWindow: {
        title: 'Floating Window',
        show: 'Show Floating Window',
      },
      selectionSearch: {
        title: 'Selection Search',
        enable: 'Selection Search',
      },
    },
    // Language options
    languages: {
      zhCN: 'Chinese',
      enUS: 'English',
    },
    // Theme options
    themes: {
      light: 'Light',
      dark: 'Dark',
      system: 'System',
    },
    // About Us
    about: {
      title: 'About Us',
      appName: 'WillChat',
      copyright: '© 2026 WillChat Sesame Network Technology  All rights reserved',
      officialWebsite: 'Official Website',
      view: 'View',
    },
    // Model service
    modelService: {
      enabled: 'Enabled',
      apiKey: 'API Key',
      apiKeyPlaceholder: 'Enter API Key',
      apiKeyRequired: 'Please enter API Key first',
      apiEndpoint: 'API Endpoint',
      apiEndpointPlaceholder: 'Enter API Endpoint',
      apiEndpointHint: 'Optional, leave empty to use default',
      apiEndpointRequired: 'Please enter API Endpoint first',
      apiVersion: 'API Version',
      apiVersionPlaceholder: 'e.g., 2024-02-01',
      apiVersionRequired: 'Please enter API Version first',
      check: 'Check',
      checkSuccess: 'Check Passed',
      checkFailed: 'Check Failed',
      reset: 'Reset',
      llmModels: 'LLM Models',
      embeddingModels: 'Embedding Models',
      noModels: 'No models',
      loadingProviders: 'Loading...',
      loadFailed: 'Failed to load',
      formIncomplete: 'Please complete required fields',
      // Model CRUD
      addModel: 'Add Model',
      editModel: 'Edit Model',
      deleteModel: 'Delete Model',
      modelId: 'Model ID',
      modelIdPlaceholder: 'Enter model ID, e.g., gpt-4o',
      modelName: 'Model Name',
      modelNamePlaceholder: 'Enter model display name',
      modelType: 'Model Type',
      cancel: 'Cancel',
      save: 'Save',
      modelCreated: 'Model created successfully',
      modelUpdated: 'Model updated successfully',
      modelDeleted: 'Model deleted successfully',
      deleteConfirmTitle: 'Confirm Delete',
      deleteConfirmMessage: 'Are you sure you want to delete the model "{name}"? This action cannot be undone.',
      confirmDelete: 'Delete',
      deleting: 'Deleting...',
    },
  },
}
