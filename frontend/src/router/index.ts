import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      redirect: '/search',
    },
    // Legacy redirects
    { path: '/profiles',   redirect: '/settings' },
    { path: '/axes',       redirect: '/search' },
    { path: '/search-log', redirect: '/search?log=open' },
    { path: '/download',   redirect: '/papers' },
    { path: '/summaries',  redirect: '/papers?has_summary=true' },
    { path: '/stats',          redirect: '/analysis' },
    { path: '/citation-graph', redirect: '/analysis' },
    // Active routes
    {
      path: '/search',
      name: 'search',
      component: () => import('../views/AxesView.vue'),
    },
    {
      path: '/papers',
      name: 'papers',
      component: () => import('../views/PapersView.vue'),
    },
    {
      path: '/analysis',
      name: 'analysis',
      component: () => import('../views/AnalysisView.vue'),
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('../views/SettingsView.vue'),
    },
  ],
})

export default router