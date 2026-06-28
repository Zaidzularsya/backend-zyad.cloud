<template>
  <section class="lp-section">
    <div class="lp-container lp-faq-layout">
      <div class="lp-section-heading lp-reveal">
        <p class="lp-eyebrow">FAQ</p>
        <h2>{{ content.title }}</h2>
      </div>
      <div class="lp-faq-list">
        <details v-for="item in content.items || []" :key="item.question" class="lp-faq-item lp-reveal">
          <summary>{{ item.question }}</summary>
          <p>{{ item.answer }}</p>
        </details>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  section: {
    type: Object,
    required: true,
  },
})

const content = computed(() => props.section.content || props.section.Content || {})
</script>
