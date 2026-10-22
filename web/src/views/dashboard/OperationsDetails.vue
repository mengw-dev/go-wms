<script setup lang="ts">
import { useRouter } from 'vue-router'
import type { DashboardOverview } from '@/api/dashboard'
import { formatTime } from '@/utils'

const props = defineProps<{ data: DashboardOverview; warehouseId: string }>()
const mode = defineModel<'' | 'activity' | 'imports'>({ required: true })
const router = useRouter()
function setOpen(value: boolean) { if (!value) mode.value = '' }
function ledger(id: string) {
  void router.push({ path: '/inventory', query: { inventory_id: id, ...(props.warehouseId ? { warehouse_id: props.warehouseId } : {}) } })
}
</script>

<template>
  <el-dialog :model-value="mode !== ''" @update:model-value="setOpen" class="business-dialog operations-details" :title="mode === 'activity' ? '今日库存作业' : '近 7 日异常导入'" width="760px" append-to-body>
    <template v-if="mode === 'activity' && data.activity">
      <p class="details-note">{{ warehouseId ? '当前所选仓库' : '全部仓库' }} · {{ data.today_from }} 至 {{ data.today_to }}（北京时间）。上架 {{ data.activity.putaway_records }} 笔、发货 {{ data.activity.shipment_records }} 笔库存流水；一次作业可能产生多笔流水，不代表完成单量或任务数。</p>
      <p class="details-note">展示最近 8 笔；“批次流水”可查看该库存记录的全部历史。</p>
      <el-table :data="data.activity.recent" max-height="360" empty-text="今日尚无上架或发货流水">
        <el-table-column label="作业" width="80"><template #default="{ row }">{{ row.trans_type === 'RECEIVE' ? '上架' : '发货' }}</template></el-table-column>
        <el-table-column prop="order_no" label="来源单号" min-width="180" show-overflow-tooltip />
        <el-table-column label="发生时间" min-width="165"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
        <el-table-column label="核对" width="100"><template #default="{ row }"><el-button link type="primary" @click="ledger(row.inventory_id)">批次流水 ↗</el-button></template></el-table-column>
      </el-table>
    </template>
    <template v-else-if="mode === 'imports' && data.import_issues">
      <p class="details-note">全部仓库 · {{ data.trend_from }} 至 {{ data.today_to }}（北京时间）创建的导入任务中，当前失败或完成后有失败行的共 {{ data.import_issues.total }} 次。此处保留历史异常，不代表尚未处理的故障；不随仓库筛选变化。</p>
      <div class="import-issue-list">
        <article v-for="item in data.import_issues.recent" :key="item.task_id" class="import-issue">
          <div><b>{{ item.file_name || item.task_id }}</b><el-tag size="small" type="warning" effect="plain">{{ item.status === 'FAILED' ? '导入失败' : '部分行失败' }}</el-tag></div>
          <p>{{ item.task_id }} · {{ formatTime(item.created_at) }}</p>
          <p>成功 {{ item.success_rows }} 行 · 失败 {{ item.fail_rows }} 行</p>
          <p class="issue-error">{{ item.error_msg || '未记录具体原因，请核对源文件与导入结果。' }}</p>
        </article>
        <p v-if="!data.import_issues.recent.length" class="details-note">当前时间范围没有异常导入记录。</p>
      </div>
      <p v-if="data.import_issues.total > 5" class="details-note">仅展示最近 5 次异常。</p>
    </template>
    <template #footer><el-button v-if="mode === 'imports'" @click="router.push('/inbound/orders')">打开入库管理 ↗</el-button><el-button @click="mode = ''">关闭</el-button></template>
  </el-dialog>
</template>

<style scoped>
.details-note { color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.8; margin: 0 0 12px; }
.import-issue-list { max-height: min(360px, 45dvh); overflow: auto; overscroll-behavior: contain; }
.import-issue { padding: 14px 0; border-bottom: 1px solid var(--el-border-color-lighter); overflow-wrap: anywhere; }
.import-issue:first-child { padding-top: 0; } .import-issue:last-child { border: 0; }
.import-issue > div { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; } .import-issue b { font-size: 13px; }
.import-issue p { font-size: 12px; color: var(--el-text-color-secondary); margin: 6px 0 0; line-height: 1.7; }
.import-issue .issue-error { color: var(--el-color-warning); white-space: pre-wrap; }
</style>
