<template>
  <section class="lp-section lp-hero">
    <div class="lp-container lp-hero-grid">
      <div class="lp-hero-copy lp-reveal">
        <p class="lp-eyebrow">{{ content.eyebrow }}</p>
        <h1>{{ content.title }}</h1>
        <p class="lp-lead">{{ content.description }}</p>
        <div class="lp-actions">
          <a class="lp-button lp-button-primary" href="#contact">{{ content.primaryCta }}</a>
          <a class="lp-button lp-button-ghost" href="#features">{{ content.secondaryCta }}</a>
        </div>
        <div class="lp-badges">
          <span v-for="badge in content.trustBadges || []" :key="badge">{{ badge }}</span>
        </div>
      </div>

      <div class="lp-hero-visual lp-reveal lp-delay-1" aria-hidden="true">
        <div class="lp-mockup">
          <div class="lp-mockup-topbar">
            <span></span><span></span><span></span>
          </div>
          <div class="lp-mockup-body">
            <div class="lp-metric-card lp-float">
              <strong>98%</strong>
              <small>Readiness score</small>
            </div>
            <div class="lp-chart-card lp-float lp-float-2">
              <span v-for="height in [46, 70, 54, 82, 64]" :key="height" :style="{ height: `${height}%` }"></span>
            </div>
            <div class="lp-panel-card lp-float lp-float-3">
              <p>{{ visualLabel }}</p>
              <div></div><div></div><div></div>
            </div>
          </div>
        </div>
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
const visualLabel = computed(() => content.value?.visual?.prompt || 'Enterprise operating system preview')
</script>
