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
// Service components are created imperatively, so their styles are not
// auto-imported by unplugin-vue-components. Import them explicitly to keep
// message boxes/messages centered and fully styled.
import 'element-plus/es/components/message-box/style/css'
import 'element-plus/es/components/message/style/css'
import 'element-plus/es/components/loading/style/css'

import App from './App.vue'
import router from './router'
import { permission } from './directives/permission'
import './style.css'
import { useThemeStore } from './stores/theme'

const app = createApp(App)

app.use(createPinia())
useThemeStore().init()
app.use(router)
app.directive('permission', permission)

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
