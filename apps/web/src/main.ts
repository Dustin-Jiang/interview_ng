import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import './assets/index.css'

// 组合式函数（useXXX()）+ 函数式风格，不使用 Pinia / 全局状态库。
const app = createApp(App)

app.use(router)

app.mount('#app')
