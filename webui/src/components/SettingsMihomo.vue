<script setup lang="ts">
import { FolderOpen, Plus, RefreshCw } from '@lucide/vue';
import { onMounted, ref } from 'vue';
import { NAlert, NButton, NCard, NForm, NFormItem, NIcon, NInput, NList, NListItem, NSpace, NTag } from 'naive-ui';
import { api } from '../api';
import type { MihomoSettings } from '../types';

const emit = defineEmits<{
  message: [value: string];
}>();

const settings = ref<MihomoSettings | null>(null);
const configDir = ref('');
const providerName = ref('');
const providerURL = ref('');
const loading = ref(false);
const adding = ref(false);
const error = ref('');

/** 应用后端返回的 Mihomo 设置快照。 */
function applySettings(value: MihomoSettings) {
  settings.value = value;
  configDir.value = value.config_dir;
}

/** 读取已保存或指定目录的 Mihomo 配置。 */
async function loadSettings(directory?: string) {
  loading.value = true;
  error.value = '';
  try {
    applySettings(await api.getMihomo(directory));
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '无法读取 Mihomo 配置';
  } finally {
    loading.value = false;
  }
}

/** 验证并保存当前 Mihomo 配置目录。 */
async function selectDirectory(directory = configDir.value) {
  loading.value = true;
  error.value = '';
  try {
    applySettings(await api.saveMihomoDirectory(directory));
    emit('message', 'Mihomo 配置目录已保存');
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '无法选择 Mihomo 配置目录';
  } finally {
    loading.value = false;
  }
}

/** 恢复并保存 Mihomo 默认配置目录。 */
async function useDefaultDirectory() {
  const defaultDirectory = settings.value?.default_config_dir;
  if (!defaultDirectory) {
    await loadSettings();
    return;
  }
  configDir.value = defaultDirectory;
  await selectDirectory(defaultDirectory);
}

/** 向当前配置追加一个 HTTP Provider。 */
async function addProvider() {
  adding.value = true;
  error.value = '';
  try {
    applySettings(
      await api.addMihomoProvider({
        config_dir: configDir.value,
        name: providerName.value,
        url: providerURL.value,
      }),
    );
    const addedName = providerName.value.trim();
    providerName.value = '';
    providerURL.value = '';
    emit('message', `Mihomo Provider ${addedName} 已添加`);
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '无法添加 Mihomo Provider';
  } finally {
    adding.value = false;
  }
}

onMounted(() => loadSettings());
</script>

<template>
  <section class="view-stack">
    <NCard :bordered="false">
      <div>
        <h2>Mihomo Provider</h2>
        <p class="muted">选择 Mihomo 配置目录，并快速追加订阅 Provider。现有 YAML 其他配置会保留。</p>
      </div>
    </NCard>

    <NAlert v-if="error" type="error" :show-icon="true">{{ error }}</NAlert>

    <NCard title="配置目录" :bordered="false">
      <NForm @submit.prevent="selectDirectory()">
        <NFormItem label="Mihomo 配置目录">
          <NInput v-model:value="configDir" placeholder="~/.config/mihomo" clearable />
        </NFormItem>
        <p v-if="settings" class="muted">配置文件：{{ settings.config_path }}</p>
        <NSpace>
          <NButton type="primary" attr-type="submit" :loading="loading">
            <template #icon><NIcon :component="FolderOpen" /></template>
            选择并载入
          </NButton>
          <NButton secondary :disabled="loading" @click="useDefaultDirectory">
            <template #icon><NIcon :component="RefreshCw" /></template>
            使用默认目录
          </NButton>
        </NSpace>
      </NForm>
    </NCard>

    <NCard title="已有 Provider" :bordered="false">
      <NList v-if="settings?.providers.length" bordered>
        <NListItem v-for="provider in settings.providers" :key="provider.name">
          <NSpace align="center" justify="space-between">
            <strong>{{ provider.name }}</strong>
            <NTag :type="provider.has_url ? 'success' : 'warning'">
              {{ provider.has_url ? '已配置 URL' : '未配置 URL' }}
            </NTag>
          </NSpace>
        </NListItem>
      </NList>
      <p v-else class="muted">当前配置中没有 Proxy Provider。</p>
    </NCard>

    <NCard title="新增 Provider" :bordered="false">
      <NForm @submit.prevent="addProvider">
        <NFormItem label="名称" required>
          <NInput v-model:value="providerName" placeholder="provider1" autocomplete="off" />
        </NFormItem>
        <NFormItem label="订阅 URL" required>
          <NInput
            v-model:value="providerURL"
            type="password"
            show-password-on="click"
            autocomplete="new-password"
            placeholder="https://example.com/subscription"
          />
        </NFormItem>
        <NButton
          type="primary"
          attr-type="submit"
          :loading="adding"
          :disabled="!configDir.trim() || !providerName.trim() || !providerURL.trim()"
        >
          <template #icon><NIcon :component="Plus" /></template>
          添加 Provider
        </NButton>
      </NForm>
    </NCard>
  </section>
</template>
