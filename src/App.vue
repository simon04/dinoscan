<template>
  <div class="vector-header-container">
    <div class="vector-header mw-header">
      <h1 @click="showDialog = true" style="cursor: pointer">DinoScan</h1>
      <span style="padding-left: 1rem">
        {{ tt("toolname") }} {{ tt("subtitle") }}
      </span>
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
      <span>
        <a
          :href="`https://tooltranslate.toolforge.org/#tool=11&languages=${language}`"
          target="_blank"
          rel="external noopener"
        >
          <cdx-icon :icon="cdxIconEdit" />
        </a>
      </span>
    </div>
  </div>

  <cdx-dialog
    v-model:open="showDialog"
    title="Wikimania 2024 Hackathon"
    :use-close-button="true"
  >
    <a
      href="https://commons.wikimedia.org/wiki/File:Wikimania_2024_Dragons_in_love_illustration_(beige_background).png"
      title="By Illustrated by Kasia Ostrowska (bazgra_nina) – Wikimedia Foundation – Logos of Wikimania 2024, CC0"
      target="_blank"
      rel="external noopener"
    >
      <img
        style="max-width: 100%; max-height: 80vh"
        src="https://upload.wikimedia.org/wikipedia/commons/d/dd/Wikimania_2024_Dragons_in_love_illustration_%28beige_background%29.png"
      />
    </a>
  </cdx-dialog>

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
      <cdx-tab name="tab_wikidata" :label="tt('wikidata')">
        <WikidataTab style="margin-top: 1rem" />
      </cdx-tab>
      <cdx-tab name="tab_output" :label="tt('output')">
        <OutputTab style="margin-top: 1rem" />
      </cdx-tab>
    </cdx-tabs>

    <div style="margin-top: 2rem">
      <cdx-button
        action="progressive"
        size="large"
        weight="primary"
        @click="doIt()"
      >
        <cdx-icon :icon="cdxIconRobot" />
        {{ tt("doit") }}
      </cdx-button>
    </div>

    <div style="margin-top: 2rem">
      <ResultsTable />
    </div>

    <div class="mw-footer-container" style="color: var(--color-subtle)">
      <footer id="footer" class="mw-footer">
        <span>DinoScan {{ VITE_BUILD_DATE }} {{ VITE_BUILD_VERSION }}</span>
        &bullet;
        <a href="https://www.gnu.org/licenses/gpl.html">GPL v3</a>
        &bullet;
        <a href="https://github.com/simon04/dinoscan">
          github.com/simon04/dinoscan
        </a>
        &bullet;
        <span>Open Source Licenses: {{ VITE_APP_DEPENDENCIES }}</span>
      </footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watchEffect } from "vue";
import {
  CdxButton,
  CdxDialog,
  CdxIcon,
  CdxMenuButton,
  CdxTab,
  CdxTabs,
} from "@wikimedia/codex";
import {
  cdxIconHelpNotice,
  cdxIconLanguage,
  cdxIconRobot,
  cdxIconEdit,
} from "@wikimedia/codex-icons";
import CategoriesTab from "./components/CategoriesTab.vue";
import OutputTab from "./components/OutputTab.vue";
import PagePropertiesTab from "./components/PagePropertiesTab.vue";
import ResultsTable from "./components/ResultsTable.vue";
import SourcesTab from "./components/SourcesTab.vue";
import TemplatesTab from "./components/TemplatesTab.vue";
import WikidataTab from "./components/WikidataTab.vue";
import tt, { isRTL, language, languages } from "./i18n/tt";
import { usePetScan } from "./usePetScan";
import { useState } from "./useState";
import { useMagicKeys, whenever } from "@vueuse/core";

const VITE_BUILD_DATE = import.meta.env.VITE_BUILD_DATE;
const VITE_BUILD_VERSION = import.meta.env.VITE_BUILD_VERSION;
const VITE_APP_DEPENDENCIES = import.meta.env.VITE_APP_DEPENDENCIES;

const state = useState();
const { url, execute } = usePetScan();
const showDialog = ref(false);

const keys = useMagicKeys();
whenever(keys["Ctrl+Enter"], () => doIt());
whenever(keys["Meta+Enter"], () => doIt());

function doIt() {
  if (state.format === "json" && state.output_compatability === "catscan") {
    execute();
  } else {
    window.open(url.value);
  }
}

watchEffect(() => (document.documentElement.lang = language.value));
watchEffect(() => (document.documentElement.dir = isRTL.value ? "rtl" : "ltr"));
watchEffect(() => {
  if (isRTL.value) {
    import("@wikimedia/codex/dist/codex.style-rtl.css");
  } else {
    import("@wikimedia/codex/dist/codex.style.css");
  }
});
</script>

<style lang="scss">
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
@media screen and (min-width: 640px) {
  .field-row {
    margin-bottom: 1rem;
    display: flex;
    gap: 1rem;
  }
  .field-row > div {
    flex: 1 1;
  }
}
</style>
