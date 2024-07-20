<template>
  <section>
    <cdx-field
      :label-icon="
        state.language === 'commons'
          ? cdxIconLogoWikimediaCommons
          : state.language === 'wikidata'
            ? cdxIconLogoWikidata
            : cdxIconLanguage
      "
    >
      <cdx-lookup
        v-model:selected="state.language"
        :menu-items="menuItems"
        :initialInputValue="
          menuItems.find((i) => i.value === state.language)?.label
        "
        @input="matrixInput = $event"
      >
        <template #menu-item="{ menuItem }">
          <strong>{{ menuItem.label }}</strong>
          (value: {{ menuItem.value }})
        </template>
      </cdx-lookup>
      <template #label>{{ tt("language") }}</template>
    </cdx-field>

    <cdx-field
      :label-icon="
        state.project === 'wikipedia'
          ? cdxIconLogoWikipedia
          : cdxIconLogoWikimedia
      "
    >
      <cdx-text-input name="project" v-model="state.project" />
      <template #label>{{ tt("project") }}</template>
    </cdx-field>

    <cdx-field>
      <cdx-text-input name="depth" v-model="state.depth" input-type="number" />
      <template #label>{{ tt("depth") }}</template>
      <template #description>
        <span v-html="tt('depth_explanation')" />
      </template>
    </cdx-field>

    <cdx-field>
      <cdx-text-area
        name="categories"
        v-model="state.categories"
        input-type="number"
      />
      <template #label>{{ tt("categories") }}</template>
    </cdx-field>

    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("combination") }}</template>
      <cdx-radio
        v-model="state.combination"
        name="combination"
        input-value="subset"
      >
        {{ tt("comb_subset") }}
      </cdx-radio>
      <cdx-radio
        v-model="state.combination"
        name="combination"
        input-value="union"
      >
        {{ tt("comb_union") }}
      </cdx-radio>
    </cdx-field>

    <cdx-field>
      <cdx-text-area
        name="negcats"
        v-model="state.negcats"
        input-type="number"
      />
      <template #label>{{ tt("negative_categories") }}</template>
    </cdx-field>
  </section>
</template>

<script setup lang="ts">
import {
  CdxField,
  CdxLookup,
  CdxRadio,
  CdxTextArea,
  CdxTextInput,
  MenuItemData,
} from "@wikimedia/codex";
import {
  cdxIconLanguage,
  cdxIconLogoWikidata,
  cdxIconLogoWikimedia,
  cdxIconLogoWikimediaCommons,
  cdxIconLogoWikipedia,
} from "@wikimedia/codex-icons";
import { computed, ref } from "vue";
import tt from "../i18n/tt";
import { useSiteMatrix } from "../useSiteMatrix";
import { useState } from "../useState";

const state = useState();
const matrixInput = ref("");
const { matrix } = useSiteMatrix();
const menuItems = computed(() =>
  matrix.value
    .filter(
      (item) => !matrixInput.value || item?.code?.startsWith(matrixInput.value),
    )
    .map(
      (item): MenuItemData => ({
        value: item.code,
        label: `${item.name} (${item.localname})`,
      }),
    )
    .concat(
      {
        value: "commons",
        label: tt("site_commons"),
      },
      {
        value: "wikidata",
        label: tt("site_wikidata"),
      },
    ),
);
</script>
