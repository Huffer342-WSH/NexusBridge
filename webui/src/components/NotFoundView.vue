<!-- 未知前端地址的轻量回退页面。 -->
<script setup lang="ts">
import { NButton, NResult } from 'naive-ui';
import { RouterLink } from 'vue-router';
</script>

<template>
  <NResult status="404" title="页面不存在" description="当前地址没有对应的 NexusBridge 页面。">
    <template #footer>
      <RouterLink v-slot="{ href, navigate }" to="/media" custom>
        <NButton tag="a" type="primary" :href="href" @click="navigate">返回媒体库</NButton>
      </RouterLink>
    </template>
  </NResult>
</template>
