<template>
  <section>
    <div class="field-row">
      <div v-for="option in options">
        <cdx-field :label-icon="cdxIconTemplateAdd">
          <cdx-text-area
            name="templates"
            v-model="state[`templates_${option}`]"
          />
          <template #label>{{ tt("templates") }}</template>
          <template #description>{{ tt(`templates_${option}`) }}</template>
        </cdx-field>
        <cdx-checkbox v-model="state[`templates_use_talk_${option}`]">
          {{ tt("templates_use_talk_instead") }}
        </cdx-checkbox>
      </div>
    </div>

    <div class="field-row">
      <div v-for="option in options">
        <cdx-field :label-icon="cdxIconLink">
          <cdx-text-area
            name="templates"
            v-model="state[`outlinks_${option}`]"
          />
          <template #label>{{ tt("outlinks") }}</template>
          <template #description>{{ tt(`outlinks_${option}`) }}</template>
        </cdx-field>
      </div>
    </div>

    <div class="field-row">
      <div v-for="option in options">
        <cdx-field :label-icon="cdxIconLink">
          <cdx-text-area
            name="templates"
            v-model="state[`links_to_${option === 'yes' ? 'any' : option}`]"
          />
          <template #label>{{ tt("links_to") }}</template>
          <template #description>{{ tt(`outlinks_${option}`) }}</template>
        </cdx-field>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { CdxCheckbox, CdxField, CdxTextArea } from "@wikimedia/codex";
import { cdxIconLink, cdxIconTemplateAdd } from "@wikimedia/codex-icons";
import tt from "../i18n/tt";
import { useState } from "../useState";

const state = useState();
const options = ["yes", "any", "no"] as const;
</script>
