import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://djangoailab.github.io',
  base: '/pve-storage-guard',
  integrations: [
    starlight({
      title: 'PVE Storage Guard',
      description: 'Adaptive I/O protection for Proxmox VE hosts',
      customCss: ['./src/styles/custom.css'],
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/DjangoAILab/pve-storage-guard' },
      ],
      sidebar: [
        { label: 'Start', items: [
          { label: 'Why Storage Guard', slug: 'index' },
          { label: 'Getting started', slug: 'getting-started' },
        ] },
        { label: 'Concepts', items: [
          { label: 'Architecture', slug: 'concepts/architecture' },
          { label: 'Policy and safety', slug: 'concepts/policy' },
          { label: 'Community context', slug: 'concepts/prior-art' },
        ] },
        { label: 'Evidence', items: [
          { label: 'Offline PoC', slug: 'evidence/poc' },
          { label: 'Replay trace contract', slug: 'evidence/trace-contract' },
          { label: 'External trace research', slug: 'evidence/external-traces' },
          { label: 'Contributing traces safely', slug: 'evidence/contributing-traces' },
          { label: 'Performance evidence', slug: 'evidence/performance' },
          { label: 'Incident case study', slug: 'evidence/case-study' },
        ] },
        { label: 'Operations', items: [
          { label: 'Read-only PVE agent', slug: 'operations/pve-agent' },
          { label: 'ITOps integration', slug: 'operations/itops' },
          { label: 'Safety gates', slug: 'operations/safety-gates' },
        ] },
        { label: '中文', items: [
          { label: '项目概览', slug: 'zh/overview' },
        ] },
      ],
      head: [
        { tag: 'meta', attrs: { name: 'theme-color', content: '#0b1215' } },
        { tag: 'meta', attrs: { name: 'keywords', content: 'Proxmox VE, PVE, disk I/O starvation, ZFS latency, adaptive throttling' } },
      ],
      lastUpdated: true,
    }),
  ],
});
