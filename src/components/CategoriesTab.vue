<template>
  <section>
    <cdx-field>
      <cdx-text-input name="language" v-model="state.language" />
      <template #label>{{ tt("language") }}</template>
    </cdx-field>

    <cdx-field>
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
  CdxRadio,
  CdxTextArea,
  CdxTextInput,
} from "@wikimedia/codex";
import tt from "../i18n/tt";
import { useState } from "../useState";
const state = useState();
</script>
