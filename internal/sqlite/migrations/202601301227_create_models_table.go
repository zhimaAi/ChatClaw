package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// 创建供应商表
			createProviders := `
create table if not exists providers (
    id integer primary key autoincrement,
    created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp,
    
    provider_id varchar(64) not null unique,
    name varchar(64) not null,
    type varchar(16) not null default 'openai',
    icon varchar(64) not null default '',
    is_builtin boolean not null default false,
    enabled boolean not null default false,
    sort_order integer not null default 0,
    
    api_endpoint varchar(1024) not null default '',
    api_key varchar(1024) not null default '',
    extra_config text not null default '{}'
);
`
			if _, err := db.ExecContext(ctx, createProviders); err != nil {
				return err
			}

			// 创建模型表
			createModels := `
create table if not exists models (
    id integer primary key autoincrement,
    created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp,
    
    provider_id varchar(64) not null,
    model_id varchar(128) not null,
    name varchar(128) not null,
    type varchar(16) not null default 'llm',
    is_builtin boolean not null default false,
    enabled boolean not null default true,
    sort_order integer not null default 0,
    
    unique(provider_id, model_id)
);
`
			if _, err := db.ExecContext(ctx, createModels); err != nil {
				return err
			}

			// 初始化内置供应商
			insertProviders := `
insert into providers (provider_id, name, type, icon, is_builtin, sort_order, api_endpoint) values
('openai', 'OpenAI', 'openai', 'openai', true, 1, 'https://api.openai.com/v1'),
('azure', 'Azure OpenAI', 'azure', 'azure', true, 2, ''),
('anthropic', 'Anthropic', 'anthropic', 'anthropic', true, 3, 'https://api.anthropic.com/v1'),
('google', 'Google Gemini', 'gemini', 'google', true, 4, 'https://generativelanguage.googleapis.com/v1beta'),
('deepseek', 'DeepSeek', 'openai', 'deepseek', true, 5, 'https://api.deepseek.com/v1'),
('zhipu', '智谱 GLM', 'openai', 'zhipu', true, 6, 'https://open.bigmodel.cn/api/paas/v4'),
('qwen', '通义千问', 'openai', 'qwen', true, 7, 'https://dashscope.aliyuncs.com/compatible-mode/v1'),
('doubao', '豆包', 'openai', 'doubao', true, 8, 'https://ark.cn-beijing.volces.com/api/v3'),
('baidu', '百度文心', 'openai', 'baidu', true, 9, 'https://qianfan.baidubce.com/v2'),
('groq', 'Groq', 'openai', 'groq', true, 10, 'https://api.groq.com/openai/v1'),
('ollama', 'Ollama', 'openai', 'ollama', true, 11, 'http://localhost:11434/v1');
`
			if _, err := db.ExecContext(ctx, insertProviders); err != nil {
				return err
			}

			// 初始化内置模型
			insertModels := `
insert into models (provider_id, model_id, name, type, is_builtin, sort_order) values
('openai', 'gpt-4o', 'GPT-4o', 'llm', true, 1),
('openai', 'gpt-4o-mini', 'GPT-4o Mini', 'llm', true, 2),
('openai', 'gpt-4-turbo', 'GPT-4 Turbo', 'llm', true, 3),
('openai', 'o1', 'o1', 'llm', true, 4),
('openai', 'o1-mini', 'o1 Mini', 'llm', true, 5),
('openai', 'text-embedding-3-small', 'Text Embedding 3 Small', 'embedding', true, 10),
('openai', 'text-embedding-3-large', 'Text Embedding 3 Large', 'embedding', true, 11),
('anthropic', 'claude-sonnet-4-20250514', 'Claude Sonnet 4', 'llm', true, 1),
('anthropic', 'claude-3-5-sonnet-20241022', 'Claude 3.5 Sonnet', 'llm', true, 2),
('anthropic', 'claude-3-5-haiku-20241022', 'Claude 3.5 Haiku', 'llm', true, 3),
('anthropic', 'claude-3-opus-20240229', 'Claude 3 Opus', 'llm', true, 4),
('google', 'gemini-2.0-flash', 'Gemini 2.0 Flash', 'llm', true, 1),
('google', 'gemini-2.0-flash-lite', 'Gemini 2.0 Flash Lite', 'llm', true, 2),
('google', 'gemini-1.5-pro', 'Gemini 1.5 Pro', 'llm', true, 3),
('google', 'gemini-1.5-flash', 'Gemini 1.5 Flash', 'llm', true, 4),
('deepseek', 'deepseek-chat', 'DeepSeek V3', 'llm', true, 1),
('deepseek', 'deepseek-reasoner', 'DeepSeek R1', 'llm', true, 2),
('zhipu', 'glm-4-plus', 'GLM-4 Plus', 'llm', true, 1),
('zhipu', 'glm-4-flash', 'GLM-4 Flash', 'llm', true, 2),
('zhipu', 'glm-4-long', 'GLM-4 Long', 'llm', true, 3),
('zhipu', 'embedding-3', 'Embedding-3', 'embedding', true, 10),
('qwen', 'qwen-max', '通义千问 Max', 'llm', true, 1),
('qwen', 'qwen-plus', '通义千问 Plus', 'llm', true, 2),
('qwen', 'qwen-turbo', '通义千问 Turbo', 'llm', true, 3),
('qwen', 'qwq-plus', 'QwQ Plus', 'llm', true, 4),
('qwen', 'text-embedding-v3', 'Text Embedding V3', 'embedding', true, 10),
('doubao', 'doubao-pro-32k', '豆包 Pro 32K', 'llm', true, 1),
('doubao', 'doubao-lite-32k', '豆包 Lite 32K', 'llm', true, 2),
('baidu', 'ernie-4.0-8k', 'ERNIE 4.0', 'llm', true, 1),
('baidu', 'ernie-3.5-8k', 'ERNIE 3.5', 'llm', true, 2),
('baidu', 'ernie-speed-8k', 'ERNIE Speed', 'llm', true, 3),
('groq', 'llama-3.3-70b-versatile', 'Llama 3.3 70B', 'llm', true, 1),
('groq', 'llama-3.1-8b-instant', 'Llama 3.1 8B', 'llm', true, 2),
('groq', 'mixtral-8x7b-32768', 'Mixtral 8x7B', 'llm', true, 3);
`
			if _, err := db.ExecContext(ctx, insertModels); err != nil {
				return err
			}

			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `drop table if exists models`); err != nil {
				return err
			}
			if _, err := db.ExecContext(ctx, `drop table if exists providers`); err != nil {
				return err
			}
			return nil
		},
	)
}
