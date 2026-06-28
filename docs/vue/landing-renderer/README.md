# Landing Renderer Vue Components

Komponen di folder ini adalah reference implementation untuk merender payload dari:

```txt
GET /api/v1/public/landing/resolve?slug={slug}
```

Repo ini belum memiliki project Vue aktif, jadi file ini disimpan sebagai komponen standalone yang bisa dipindahkan ke frontend tenant/public renderer.

## Expected Input

`LandingPageRenderer.vue` menerima object resolver backend. Backend saat ini memakai field Go tanpa JSON tag untuk `ResolvedPage`, sehingga komponen menerima dua bentuk:

- `Page`, `Sections`, `Branding`, `Snapshot`
- `page`, `sections`, `branding`, `snapshot`

Section yang didukung oleh seed `000038`:

- `hero`
- `partner_logos`
- `features`
- `statistics`
- `services`
- `pricing`
- `faq`
- `cta`

## Usage

```vue
<template>
  <LandingPageRenderer :resolved="resolvedPage" />
</template>

<script setup>
import LandingPageRenderer from './components/LandingPageRenderer.vue'

defineProps({
  resolvedPage: {
    type: Object,
    required: true,
  },
})
</script>
```
