<script setup lang="ts">
import { Network, RotateCcw } from '@lucide/vue';
import { NButton, NCard, NForm, NFormItem, NIcon, NInput, NRadio, NRadioGroup } from 'naive-ui';
import type { NetworkConfig } from '../types';

defineProps<{
  config: NetworkConfig;
}>();

const emit = defineEmits<{
  update: [patch: Partial<NetworkConfig>];
  save: [];
}>();

const defaultNoProxy = `localhost
127.*
192.168.*
10.*
172.16.*
172.17.*
172.18.*
172.19.*
172.20.*
172.21.*
172.22.*
172.23.*
172.24.*
172.25.*
172.26.*
172.27.*
172.28.*
172.29.*
172.30.*
172.31.*
dl.steam.clngaa.com
st.dl.eccdnx.com
*.bilibili.com
*.bilivideo.com
*yuanshen.com
ug.local`;
</script>

<template>
  <section class="view-stack">
    <NCard :bordered="false">
      <div>
        <h2>网络代理</h2>
        <p class="muted">应用通过标准代理环境变量配置网络访问；qBittorrent 地址会自动追加到 NO_PROXY。</p>
      </div>
    </NCard>

    <NCard :bordered="false">
      <NForm class="settings-form" @submit.prevent="emit('save')">
        <NFormItem label="代理模式">
          <NRadioGroup
            :value="config.mode"
            name="network-proxy-mode"
            @update:value="(value) => emit('update', { mode: value as NetworkConfig['mode'] })"
          >
            <NRadio value="system">系统代理</NRadio>
            <NRadio value="manual">手动代理</NRadio>
            <NRadio value="direct">直连</NRadio>
          </NRadioGroup>
        </NFormItem>
        <NFormItem v-if="config.mode === 'manual'" label="代理地址">
          <NInput
            :value="config.proxy_url"
            placeholder="http://127.0.0.1:7890 或 socks5://127.0.0.1:7891"
            autocomplete="off"
            @update:value="(value) => emit('update', { proxy_url: value })"
          />
        </NFormItem>
        <NFormItem label="NO_PROXY">
          <NInput
            :value="config.no_proxy"
            type="textarea"
            :autosize="{ minRows: 8, maxRows: 20 }"
            placeholder="一行一项，例如 192.168.*"
            @update:value="(value) => emit('update', { no_proxy: value })"
          />
        </NFormItem>
        <NButton secondary @click="emit('update', { no_proxy: defaultNoProxy })">
          <template #icon>
            <NIcon :component="RotateCcw" />
          </template>
          恢复默认 NO_PROXY
        </NButton>
        <p class="muted">规则一行一项；保存后需重启 NexusBridge，Go 标准库才会重新读取代理环境变量。</p>
        <NButton type="primary" attr-type="submit">
          <template #icon>
            <NIcon :component="Network" />
          </template>
          保存网络设置
        </NButton>
      </NForm>
    </NCard>
  </section>
</template>
