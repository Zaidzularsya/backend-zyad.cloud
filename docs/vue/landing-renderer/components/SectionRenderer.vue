<template>
  <component :is="component" :section="section" />
</template>

<script setup>
import { computed } from 'vue'
import AnimatedHeroSection from './sections/AnimatedHeroSection.vue'
import TrustStripSection from './sections/TrustStripSection.vue'
import FeatureGridSection from './sections/FeatureGridSection.vue'
import StatisticsSection from './sections/StatisticsSection.vue'
import ProcessSection from './sections/ProcessSection.vue'
import PricingSection from './sections/PricingSection.vue'
import FaqSection from './sections/FaqSection.vue'
import CtaSection from './sections/CtaSection.vue'
import DynamicSectionFallback from './sections/DynamicSectionFallback.vue'

const props = defineProps({
  section: {
    type: Object,
    required: true,
  },
})

const sectionType = computed(() => props.section.type || props.section.Type || props.section.section_type || props.section.sectionType)

const component = computed(() => {
  switch (sectionType.value) {
    case 'hero':
      return AnimatedHeroSection
    case 'partner_logos':
      return TrustStripSection
    case 'features':
      return FeatureGridSection
    case 'statistics':
      return StatisticsSection
    case 'services':
      return ProcessSection
    case 'pricing':
      return PricingSection
    case 'faq':
      return FaqSection
    case 'cta':
      return CtaSection
    default:
      return DynamicSectionFallback
  }
})
</script>
