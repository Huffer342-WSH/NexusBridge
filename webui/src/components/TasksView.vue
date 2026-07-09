<script setup lang="ts">
import { Activity, FolderKanban, HardDriveDownload } from '@lucide/vue';
import { NAlert, NButton, NCard, NEmpty, NGrid, NGi, NIcon, NSpace, NTag, NThing } from 'naive-ui';
import type { DownloadTask, OrganizeTask } from '../types';

defineProps<{
  downloadTasks: DownloadTask[];
  organizeTasks: OrganizeTask[];
  activeDownloads: number;
  pendingOrganize: number;
}>();

const emit = defineEmits<{
  organize: [];
}>();

function statusType(status: string): 'default' | 'success' | 'warning' | 'error' | 'info' {
  const normalized = status.toLowerCase();
  if (normalized.includes('fail') || normalized.includes('error')) {
    return 'error';
  }
  if (normalized.includes('pending') || normalized.includes('running')) {
    return 'warning';
  }
  if (normalized.includes('done') || normalized.includes('complete') || normalized.includes('ok')) {
    return 'success';
  }
  return 'info';
}
</script>

<template>
  <section class="view-stack">
    <NGrid :cols="2" :x-gap="14" :y-gap="14" responsive="screen">
      <NGi span="2 m:1">
        <NCard :bordered="false" title="Download Tasks">
          <template #header-extra>
            <NTag size="small" round>{{ activeDownloads }} active</NTag>
          </template>
          <NSpace v-if="downloadTasks.length" vertical :size="8">
            <NThing v-for="task in downloadTasks" :key="task.id" class="task-item">
              <template #avatar>
                <NIcon :component="HardDriveDownload" />
              </template>
              <template #header>
                <span class="text-break">{{ task.torrent_title }}</span>
              </template>
              <template #header-extra>
                <NTag :type="statusType(task.status)" size="small" round>{{ task.status }}</NTag>
              </template>
              <p class="muted text-break">{{ task.rule_name || task.qb_hash || task.download_url }}</p>
              <NAlert v-if="task.error" type="error" :bordered="false" class="task-error">
                {{ task.error }}
              </NAlert>
            </NThing>
          </NSpace>
          <NEmpty v-else description="No download tasks" />
        </NCard>
      </NGi>

      <NGi span="2 m:1">
        <NCard :bordered="false" title="Organize Tasks">
          <template #header-extra>
            <NSpace align="center">
              <NTag size="small" round>{{ pendingOrganize }} pending</NTag>
              <NButton secondary size="small" @click="emit('organize')">
                <template #icon>
                  <NIcon :component="FolderKanban" />
                </template>
                Process
              </NButton>
            </NSpace>
          </template>
          <NSpace v-if="organizeTasks.length" vertical :size="8">
            <NThing v-for="task in organizeTasks" :key="task.id" class="task-item">
              <template #avatar>
                <NIcon :component="Activity" />
              </template>
              <template #header>
                <span class="text-break">{{ task.title }}</span>
              </template>
              <template #header-extra>
                <NTag :type="statusType(task.status)" size="small" round>{{ task.status }}</NTag>
              </template>
              <p class="muted text-break">{{ task.target_path || task.source_path }}</p>
              <NAlert v-if="task.error" type="error" :bordered="false" class="task-error">
                {{ task.error }}
              </NAlert>
            </NThing>
          </NSpace>
          <NEmpty v-else description="No organize tasks" />
        </NCard>
      </NGi>
    </NGrid>
  </section>
</template>
