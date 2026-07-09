<script setup lang="ts">
import { RefreshCw, Save } from '@lucide/vue';
import { NButton, NCard, NForm, NFormItem, NGi, NGrid, NIcon, NInput, NInputNumber, NSelect, NSpace, NSwitch, NTag } from 'naive-ui';
import { QB_POLLING_LIMITS } from '../config/qbittorrent';
import type { QBittorrentConfig } from '../types';

defineProps<{
  config: QBittorrentConfig;
  tagsText: string;
	connected: boolean | null;
	polling: boolean;
}>();

const emit = defineEmits<{
  update: [patch: Partial<QBittorrentConfig>];
  updateTags: [value: string];
  save: [];
  sync: [];
}>();

const authModeOptions = [
  { label: 'User ID / Password', value: 'uid' },
  { label: 'API Key', value: 'api_key' },
];
</script>

<template>
  <section class="view-stack">
    <NCard :bordered="false">
      <div>
        <h2>qBittorrent 设置</h2>
        <p class="muted">配置下载器连接、认证方式、分类和标签。</p>
      </div>
    </NCard>

    <NCard :bordered="false">
      <NForm class="settings-form" @submit.prevent="emit('save')">
        <NFormItem label="Auth Mode">
          <NSelect
            :value="config.auth_mode"
            :options="authModeOptions"
            @update:value="(value) => emit('update', { auth_mode: value })"
          />
        </NFormItem>
        <NFormItem label="WebUI URL">
          <NInput
            :value="config.url"
            placeholder="http://127.0.0.1:8080"
            @update:value="(value) => emit('update', { url: value })"
          />
        </NFormItem>
        <NFormItem v-if="config.auth_mode === 'api_key'" label="API Key">
          <NInput
            :value="config.api_key"
            type="password"
            show-password-on="click"
            autocomplete="off"
            placeholder="Leave blank to keep existing"
            @update:value="(value) => emit('update', { api_key: value })"
          />
        </NFormItem>
        <NGrid v-if="config.auth_mode === 'uid'" :cols="2" :x-gap="10" responsive="screen">
          <NGi>
            <NFormItem label="User ID">
              <NInput
                :value="config.user_id"
                autocomplete="username"
                @update:value="(value) => emit('update', { user_id: value })"
              />
            </NFormItem>
          </NGi>
          <NGi>
            <NFormItem label="Username">
              <NInput
                :value="config.username"
                autocomplete="username"
                @update:value="(value) => emit('update', { username: value })"
              />
            </NFormItem>
          </NGi>
        </NGrid>
        <NFormItem v-if="config.auth_mode === 'uid'" label="Password">
          <NInput
            :value="config.password"
            type="password"
            show-password-on="click"
            autocomplete="new-password"
            placeholder="Leave blank to keep existing"
            @update:value="(value) => emit('update', { password: value })"
          />
        </NFormItem>
        <NGrid :cols="2" :x-gap="10" responsive="screen">
          <NGi>
            <NFormItem label="Category">
              <NInput :value="config.category" @update:value="(value) => emit('update', { category: value })" />
            </NFormItem>
          </NGi>
          <NGi>
            <NFormItem label="Tags">
              <NInput :value="tagsText" placeholder="tag1, tag2" @update:value="emit('updateTags', $event)" />
            </NFormItem>
          </NGi>
        </NGrid>
		<NCard size="small" title="状态自动刷新" class="qb-poll-settings">
			<template #header-extra>
				<NTag :type="connected === true ? 'success' : connected === false ? 'error' : 'default'">
					{{ polling ? '同步中' : connected === true ? '已连接' : connected === false ? '未连接' : '未检测' }}
				</NTag>
			</template>
			<NFormItem label="启用 qB 增量刷新">
				<NSwitch :value="config.auto_sync" @update:value="emit('update', { auto_sync: $event })" />
			</NFormItem>
			<NGrid :cols="3" :x-gap="10" responsive="screen">
				<NGi>
					<NFormItem label="前台间隔（秒）">
						<NInputNumber :value="config.sync_interval_seconds" :min="QB_POLLING_LIMITS.syncIntervalSeconds.min" :max="QB_POLLING_LIMITS.syncIntervalSeconds.max" @update:value="emit('update', { sync_interval_seconds: $event ?? QB_POLLING_LIMITS.syncIntervalSeconds.default })" />
					</NFormItem>
				</NGi>
				<NGi>
					<NFormItem label="后台间隔（秒）">
						<NInputNumber :value="config.inactive_sync_interval_seconds" :min="QB_POLLING_LIMITS.inactiveSyncIntervalSeconds.min" :max="QB_POLLING_LIMITS.inactiveSyncIntervalSeconds.max" @update:value="emit('update', { inactive_sync_interval_seconds: $event ?? QB_POLLING_LIMITS.inactiveSyncIntervalSeconds.default })" />
					</NFormItem>
				</NGi>
				<NGi>
					<NFormItem label="断连重试（秒）">
						<NInputNumber :value="config.disconnected_sync_interval_seconds" :min="QB_POLLING_LIMITS.disconnectedSyncIntervalSeconds.min" :max="QB_POLLING_LIMITS.disconnectedSyncIntervalSeconds.max" @update:value="emit('update', { disconnected_sync_interval_seconds: $event ?? QB_POLLING_LIMITS.disconnectedSyncIntervalSeconds.default })" />
					</NFormItem>
				</NGi>
			</NGrid>
		</NCard>
        <NSpace>
          <NButton type="primary" attr-type="submit">
            <template #icon>
              <NIcon :component="Save" />
            </template>
            Save qBittorrent
          </NButton>
          <NButton secondary @click="emit('sync')">
            <template #icon>
              <NIcon :component="RefreshCw" />
            </template>
            Sync
          </NButton>
        </NSpace>
      </NForm>
    </NCard>
  </section>
</template>
