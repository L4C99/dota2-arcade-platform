import { createApp } from 'vue'
import App from './App.vue'
import AdminApp from './AdminApp.vue'
import './style.css'
import './admin.css'

createApp(window.location.pathname === '/admin' ? AdminApp : App).mount('#app')
