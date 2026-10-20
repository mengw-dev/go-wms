<script setup lang="ts">
import PageHeader from './PageHeader.vue'
import BusinessFilters from './BusinessFilters.vue'

defineProps<{ title: string; description: string; total: number }>()
</script>

<template>
  <div class="app-page business-list">
    <PageHeader :title="title" :description="description">
      <template v-if="$slots.actions" #actions><slot name="actions" /></template>
    </PageHeader>
    <section class="app-card app-card--flush">
      <BusinessFilters v-if="$slots.filters"><slot name="filters" /></BusinessFilters>
      <div class="app-table-region">
        <div class="app-toolbar">
          <span class="table-summary">{{ title }} · 共 <strong>{{ total }}</strong> 条</span>
        </div>
        <slot />
      </div>
    </section>
  </div>
</template>
