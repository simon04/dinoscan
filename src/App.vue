<template>
  <div class="vector-header-container">
    <div class="vector-header mw-header">
      <h1>{{ tt("toolname") }}</h1>
      <span style="padding-left: 1rem">{{ tt("subtitle") }}</span>
      <span style="padding-left: 1rem">Codex frontend by simon04</span>
    </div>
  </div>
  <div class="mw-page-container">
    <cdx-tabs v-model:active="currentTab">
      <cdx-tab name="categories" :label="tt('categories')">
        <CategoriesTab style="margin-top: 1rem" />
      </cdx-tab>
      <cdx-tab name="page_props" :label="tt('page_props')"> </cdx-tab>
      <cdx-tab name="templates_links" :label="tt('templates_links')"> </cdx-tab>
      <cdx-tab name="other_sources" :label="tt('other_sources')"> </cdx-tab>
      <cdx-tab name="wikidata" :label="tt('wikidata')"> </cdx-tab>
      <cdx-tab name="output" :label="tt('output')"> </cdx-tab>
    </cdx-tabs>
  </div>
</template>

<script setup lang="ts">
import { CdxTab, CdxTabs } from "@wikimedia/codex";
import { ref } from "vue";
import CategoriesTab from "./components/CategoriesTab.vue";
import tt from "./i18n/tt";

const currentTab = ref("categories");
</script>

<style lang="scss">
@import "@wikimedia/codex/dist/codex.style.css";
@import "@wikimedia/codex-design-tokens/theme-wikimedia-ui.scss";
// :root {
//   --size-0: 0;
//   --size-6: 0.0625rem;
//   --size-12: 0.125rem;
//   --size-25: 0.25rem;
//   --size-50: 0.5rem;
//   --size-75: 0.75rem;
//   --size-100: 1rem;
//   --size-125: 1.25rem;
//   --size-150: 1.5rem;
//   --size-200: 2rem;
//   --size-250: 2.5rem;
//   --size-275: 2.75rem;
//   --size-300: 3rem;
//   --size-400: 4rem;
//   --size-800: 8rem;
//   --size-1600: 16rem;
//   --size-2400: 24rem;
//   --size-2800: 28rem;
//   --size-3200: 32rem;
//   --size-4000: 40rem;
//   --size-5600: 56rem;
// }
</style>
