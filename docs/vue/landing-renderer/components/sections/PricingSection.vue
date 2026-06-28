<template>
  <section class="lp-section">
    <div class="lp-container">
      <div class="lp-section-heading lp-reveal">
        <p class="lp-eyebrow">Offer architecture</p>
        <h2>{{ content.title }}</h2>
      </div>
      <div class="lp-pricing-grid">
        <article v-for="plan in content.plans || []" :key="plan.name" class="lp-card lp-price-card lp-reveal">
          <h3>{{ plan.name }}</h3>
          <strong>{{ plan.priceLabel }}</strong>
          <ul>
            <li v-for="feature in plan.features || []" :key="feature">{{ feature }}</li>
          </ul>
        </article>
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
