import { createApp } from 'vue'
import { createPinia } from 'pinia'
import {
  ArrowDown,
  Box,
  Clock,
  Coin,
  Delete,
  Download,
  List,
  Lock,
  Moon,
  Odometer,
  OfficeBuilding,
  Plus,
  Setting,
  Sunny,
  Tickets,
  Upload,
  UploadFilled,
  User,
} from '@element-plus/icons-vue'
import 'element-plus/theme-chalk/dark/css-vars.css'

import App from './App.vue'
import router from './router'
import './style.css'
import { useThemeStore } from './stores/theme'

const app = createApp(App)

app.use(createPinia())
useThemeStore().init()
app.use(router)

const icons = {
  ArrowDown,
  Box,
  Clock,
  Coin,
  Delete,
  Download,
  List,
  Lock,
  Moon,
  Odometer,
  OfficeBuilding,
  Plus,
  Setting,
  Sunny,
  Tickets,
  Upload,
  UploadFilled,
  User,
}

for (const [key, component] of Object.entries(icons)) {
  app.component(key, component)
}

app.mount('#app')
