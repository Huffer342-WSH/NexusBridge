<script setup lang="ts">
import { KeyRound, Play, RefreshCw, Save } from '@lucide/vue';
import { NButton, NCard, NEmpty, NForm, NFormItem, NIcon, NInput, NSpace, NTag, NThing } from 'naive-ui';
import type { Site } from '../types';

type CredentialDraft = { user_agent: string; cookie: string };

defineProps<{
  sites: Site[];
  credentials: Record<string, CredentialDraft>;
  actions: Record<string, 'fetch' | 'run' | 'save' | ''>;
}>();

const emit = defineEmits<{
  updateCredential: [siteID: string, patch: Partial<CredentialDraft>];
  save: [site: Site];
  fetch: [siteID: string];
  run: [siteID: string];
}>();
</script>

<template>
  <section class="view-stack">
    <NCard :bordered="false">
      <div>
        <h2>站点设置</h2>
        <p class="muted">管理站点 Cookie、User-Agent，并触发抓取或单次运行。</p>
      </div>
    </NCard>

    <NSpace v-if="sites.length" vertical :size="12">
      <NCard v-for="site in sites" :key="site.id" :bordered="false" class="settings-site-card">
        <NThing :title="site.name">
          <template #avatar>
            <NIcon :component="KeyRound" size="20" />
          </template>
          <template #header-extra>
            <NTag :type="site.has_cookie ? 'success' : 'warning'" size="small" round>
              {{ site.has_cookie ? 'Cookie saved' : 'Cookie required' }}
            </NTag>
          </template>
          <p class="muted text-break">{{ site.base_url }}</p>
        </NThing>

        <NForm class="compact-form" @submit.prevent="emit('save', site)">
          <NFormItem label="User-Agent">
            <NInput
              :value="credentials[site.id]?.user_agent ?? ''"
              @update:value="(value) => emit('updateCredential', site.id, { user_agent: value })"
            />
          </NFormItem>
          <NFormItem label="Cookie">
            <NInput
              :value="credentials[site.id]?.cookie ?? ''"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 5 }"
              placeholder="name=value; name2=value2"
              @update:value="(value) => emit('updateCredential', site.id, { cookie: value })"
            />
          </NFormItem>
          <NSpace>
            <NButton secondary type="primary" attr-type="submit" :loading="actions[site.id] === 'save'">
              <template #icon>
                <NIcon :component="Save" />
              </template>
              Save
            </NButton>
            <NButton
              secondary
              :disabled="!!actions[site.id]"
              :loading="actions[site.id] === 'fetch'"
              @click="emit('fetch', site.id)"
            >
              <template #icon>
                <NIcon :component="RefreshCw" />
              </template>
              Fetch
            </NButton>
            <NButton
              type="primary"
              :disabled="!!actions[site.id]"
              :loading="actions[site.id] === 'run'"
              @click="emit('run', site.id)"
            >
              <template #icon>
                <NIcon :component="Play" />
              </template>
              Run
            </NButton>
          </NSpace>
        </NForm>
      </NCard>
    </NSpace>
    <NCard v-else :bordered="false">
      <NEmpty description="No sites configured" />
    </NCard>
  </section>
</template>
