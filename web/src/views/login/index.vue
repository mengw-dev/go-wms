<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { login } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({
  username: 'admin',
  password: 'admin123',
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    const result = await login({ username: form.username, password: form.password })
    auth.setAuth(result)
    ElMessage.success('登录成功')
    const redirect = route.query.redirect
    router.push(typeof redirect === 'string' && redirect.startsWith('/') ? redirect : '/')
  } finally {
    loading.value = false
  }
}

const highlights = [
  { title: '三数量库存模型', desc: '存量 = 可用 + 分配，全链路流水可追溯' },
  { title: '双重防超卖', desc: '行锁 + 条件更新，并发分配数据强一致' },
  { title: 'FIFO 智能分配', desc: '审核即锁库，先入先出跨批次扣减' },
  { title: '全流程状态机', desc: '入库 / 出库 / 盘点 / 任务，流转严格可控' },
]
</script>

<template>
  <div class="login-page">
    <!-- 左侧品牌区 -->
    <div class="brand">
      <div class="brand-head">
        <div class="brand-logo">W</div>
        <div>
          <b>GoWMS</b>
          <small>轻量级仓储管理系统</small>
        </div>
      </div>
      <div class="brand-body">
        <h1 class="brand-title">让每一个库存数字<br />都值得信赖</h1>
        <ul class="brand-list">
          <li v-for="h in highlights" :key="h.title">
            <span class="dot"></span>
            <div>
              <b>{{ h.title }}</b>
              <p>{{ h.desc }}</p>
            </div>
          </li>
        </ul>
      </div>
      <div class="brand-foot">Go · Gin · GORM · MySQL · Redis · Vue3 · TypeScript</div>
    </div>

    <!-- 右侧表单区 -->
    <div class="panel">
      <div class="login-box">
        <div class="mini-logo">W</div>
        <h2>欢迎登录</h2>
        <p class="sub">输入账号进入工作台</p>
        <el-alert type="info" :closable="false" show-icon title="默认账号：admin / admin123" class="tip" />
        <el-form ref="formRef" :model="form" :rules="rules" size="large" @keyup.enter="submit">
          <el-form-item prop="username">
            <el-input v-model="form.username" placeholder="用户名">
              <template #prefix><el-icon><User /></el-icon></template>
            </el-input>
          </el-form-item>
          <el-form-item prop="password">
            <el-input v-model="form.password" type="password" show-password placeholder="密码">
              <template #prefix><el-icon><Lock /></el-icon></template>
            </el-input>
          </el-form-item>
          <el-button type="primary" size="large" class="login-btn" :loading="loading" @click="submit">
            登 录
          </el-button>
        </el-form>
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  height: 100%;
  display: flex;
  overflow: hidden;
}

/* ---------- 左侧品牌区 ---------- */
.brand {
  flex: 1.2;
  min-width: 0;
  background: var(--gowms-login-brand-bg);
  color: #fff;
  padding: 56px 64px;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
}

/* 装饰光斑 */
.brand::before,
.brand::after {
  content: '';
  position: absolute;
  border-radius: 50%;
  pointer-events: none;
}

.brand::before {
  width: 420px;
  height: 420px;
  right: -140px;
  bottom: -160px;
  background: radial-gradient(circle, rgba(34, 211, 238, 0.18), transparent 70%);
}

.brand::after {
  width: 320px;
  height: 320px;
  left: -100px;
  top: -100px;
  background: radial-gradient(circle, rgba(255, 255, 255, 0.08), transparent 70%);
}

.brand-head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

/* 中部内容块：与右侧登录卡垂直居中对齐 */
.brand-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 32px 0;
  position: relative;
  z-index: 1;
}

.brand-logo {
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.16);
  backdrop-filter: blur(4px);
  border: 1px solid rgba(255, 255, 255, 0.25);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 800;
  font-size: 20px;
}

.brand-head b {
  font-size: 18px;
  letter-spacing: 0.5px;
  display: block;
}

.brand-head small {
  font-size: 12px;
  opacity: 0.75;
}

.brand-title {
  font-size: clamp(24px, 2.6vw, 34px);
  line-height: 1.45;
  margin: 0 0 32px;
  font-weight: 700;
}

.brand-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 18px;
  max-width: 460px;
}

.brand-list li {
  display: flex;
  gap: 12px;
}

.brand-list .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-top: 7px;
  background: #22d3ee;
  box-shadow: 0 0 8px rgba(34, 211, 238, 0.8);
  flex-shrink: 0;
}

.brand-list b {
  font-size: 14px;
}

.brand-list p {
  margin: 2px 0 0;
  font-size: 12px;
  opacity: 0.72;
}

.brand-foot {
  margin-top: auto;
  font-size: 12px;
  opacity: 0.6;
  letter-spacing: 0.5px;
}

/* ---------- 右侧表单区 ---------- */
.panel {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--el-bg-color-page);
  padding: 24px;
}

.login-box {
  width: 380px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 16px;
  padding: 36px 32px;
  box-shadow: var(--el-box-shadow);
}

.login-box h2 {
  margin: 0 0 4px;
  font-size: 22px;
  color: var(--el-text-color-primary);
}

.login-box .sub {
  margin: 0 0 20px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.tip {
  margin-bottom: 20px;
}

.login-btn {
  width: 100%;
  margin-top: 4px;
  letter-spacing: 4px;
}

/* 小屏隐藏品牌区，登录卡内显示品牌标识 */
@media (max-width: 900px) {
  .brand {
    display: none;
  }

  .mini-logo {
    display: flex;
  }
}
</style>
