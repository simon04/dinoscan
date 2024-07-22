<template>
  <div class="vector-header-container">
    <div class="vector-header mw-header">
      <h1>{{ tt("toolname") }}</h1>
      <span style="padding-left: 1rem">{{ tt("subtitle") }}</span>
      <span style="padding-left: 1rem">Codex frontend by simon04</span>
      <span style="padding-left: 1rem">
        <a
          href="https://meta.wikimedia.org/wiki/PetScan/en"
          target="_blank"
          rel="external noopener"
        >
          <cdx-icon :icon="cdxIconHelpNotice" />
        </a>
      </span>
      <span style="padding-left: 1rem">
        <cdx-menu-button v-model:selected="language" :menu-items="languages">
          <cdx-icon :icon="cdxIconLanguage" />
          {{ language }}
        </cdx-menu-button>
      </span>
    </div>
  </div>
  <div class="mw-page-container">
    <cdx-tabs v-model:active="state.active_tab">
      <cdx-tab name="tab_categories" :label="tt('categories')">
        <CategoriesTab style="margin-top: 1rem" />
      </cdx-tab>
      <cdx-tab name="tab_page_props" :label="tt('page_props')">
        <PagePropertiesTab style="margin-top: 1rem" />
      </cdx-tab>
      <cdx-tab name="tab_templates_links" :label="tt('templates_links')">
        <TemplatesTab style="margin-top: 1rem" />
      </cdx-tab>
      <cdx-tab name="tab_other_sources" :label="tt('other_sources')">
        <SourcesTab style="margin-top: 1rem" />
      </cdx-tab>
      <cdx-tab name="tab_wikidata" :label="tt('wikidata')"></cdx-tab>
      <cdx-tab name="tab_output" :label="tt('output')">
        <OutputTab style="margin-top: 1rem" />
      </cdx-tab>
    </cdx-tabs>

    <div style="margin-top: 2rem">
      <cdx-button action="progressive" size="large" weight="primary" @click="execute">
        <cdx-icon :icon="cdxIconRobot" />
        {{ tt("doit") }}
      </cdx-button>
    </div>

    <div style="margin-top: 2rem">
      <ResultsTable />
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  CdxButton,
  CdxIcon,
  CdxMenuButton,
  CdxTab,
  CdxTabs,
} from "@wikimedia/codex";
import { cdxIconHelpNotice, cdxIconLanguage, cdxIconRobot } from "@wikimedia/codex-icons";
import CategoriesTab from "./components/CategoriesTab.vue";
import OutputTab from "./components/OutputTab.vue";
import PagePropertiesTab from "./components/PagePropertiesTab.vue";
import ResultsTable from "./components/ResultsTable.vue";
import SourcesTab from "./components/SourcesTab.vue";
import TemplatesTab from "./components/TemplatesTab.vue";
import tt, { language, languages } from "./i18n/tt";
import { usePetScan } from "./usePetScan";
import { useState } from "./useState";

const state = useState();
const { execute } = usePetScan();
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
.field-row {
  margin-bottom: 1rem;
  display: flex;
  gap: 1rem;
}
.field-row > div {
  flex: 1 1;
}
</style>
