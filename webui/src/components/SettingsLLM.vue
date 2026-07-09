<script setup lang="ts">
import { Bot, Save } from '@lucide/vue';
import { NButton, NCard, NForm, NFormItem, NIcon, NInput } from 'naive-ui';
import type { LLMConfig } from '../types';

defineProps<{
  config: LLMConfig;
}>();

const emit = defineEmits<{
  update: [patch: Partial<LLMConfig>];
  save: [];
}>();
</script>

<template>
  <section class="view-stack">
    <NCard :bordered="false">
      <div>
        <h2>LLM 设置</h2>
        <p class="muted">配置标题提取和整理任务使用的兼容 OpenAI 接口。</p>
      </div>
    </NCard>

    <NCard :bordered="false">
      <NForm class="settings-form" @submit.prevent="emit('save')">
        <NFormItem label="Base URL">
          <NInput
            :value="config.base_url"
            placeholder="https://example.com/v1"
            @update:value="(value) => emit('update', { base_url: value })"
          />
        </NFormItem>
        <NFormItem label="API Key">
          <NInput
            :value="config.api_key"
            type="password"
            show-password-on="click"
            autocomplete="off"
            placeholder="Leave blank to keep existing"
            @update:value="(value) => emit('update', { api_key: value })"
          />
        </NFormItem>
        <NFormItem label="Model">
          <NInput
            :value="config.model"
            placeholder="glm-4-flash-250414"
            @update:value="(value) => emit('update', { model: value })"
          />
        </NFormItem>
        <NButton type="primary" attr-type="submit">
          <template #icon>
            <NIcon :component="Bot" />
          </template>
          Save LLM
        </NButton>
      </NForm>
    </NCard>
  </section>
</template>
