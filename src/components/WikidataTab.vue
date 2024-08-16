<template>
  <section>
    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("pages_with_items") }}</template>
      <cdx-radio
        v-for="option in wikidataItemOptions"
        v-model="state.wikidata_item"
        :name="option"
        :input-value="option"
      >
        {{ tt(`wikidata_${option}`) }}
      </cdx-radio>
    </cdx-field>

    <cdx-field style="margin-top: 2rem">
      <cdx-text-input
        name="wikidata_label_language"
        v-model="state.wikidata_label_language"
      />
      <template #label>{{ tt("wikidata_label_language") }}</template>
      <template #description>
        <span v-html="tt('wikidata_label_language_ph')" />
      </template>
    </cdx-field>

    <cdx-field style="margin-top: 2rem">
      <cdx-text-input
        name="wikidata_prop_item_use"
        v-model="state.wikidata_prop_item_use"
      />
      <template #label>{{ tt("wikidata_prop_item_use") }}</template>
      <template #description>
        <span v-html="tt('wikidata_prop_item_use_ph')" />
      </template>
      <cdx-radio
        v-for="option in wpiuOptions"
        v-model="state.wpiu"
        :name="option"
        :input-value="option"
        :inline="true"
        style="margin-top: 0.5rem"
      >
        {{ tt(`wpiu_uses_${option}`) }}
      </cdx-radio>
      <cdx-checkbox
        v-model="state.wpiu_no_statements"
        name="wpiu_no_statements"
        :inline="true"
        style="margin-top: 0.5rem"
      >
        {{ tt("wpiu_no_statements") }}
      </cdx-checkbox>
      <cdx-checkbox
        v-model="state.wpiu_no_sitelinks"
        name="wpiu_no_sitelinks"
        :inline="true"
        style="margin-top: 0.5rem"
      >
        {{ tt("wpiu_no_sitelinks") }}
      </cdx-checkbox>
    </cdx-field>

    <p style="margin-top: 2rem" v-html="tt('sitelink_note')"></p>
    <div class="field-row">
      <div v-for="option in options">
        <cdx-field :label-icon="cdxIconLink">
          <cdx-text-area
            name="templates"
            v-model="state[`sitelinks_${option}`]"
          />
          <template #label>{{ tt("sitelinks") }}</template>
          <template #description>
            {{ tt(`sitelinks_${option}`) }}
            <br />
            {{ tt("ph_sites") }}
          </template>
        </cdx-field>
      </div>
    </div>
    <div class="field-row">
      <div>
        <cdx-field :label-icon="cdxIconLink">
          <cdx-text-input
            name="larger"
            v-model="state.min_sitelink_count"
            input-type="number"
          />
          <template #label>{{ tt("link_number") }}</template>
          <template #description>{{ tt("min_sitelink_count") }}</template>
        </cdx-field>
      </div>
      <div>
        <cdx-field :label-icon="cdxIconLink">
          <cdx-text-input
            name="smaller"
            v-model="state.max_sitelink_count"
            input-type="number"
          />
          <template #label>{{ tt("link_number") }}</template>
          <template #description>{{ tt("max_sitelink_count") }}</template>
        </cdx-field>
      </div>
    </div>

    <p style="margin-top: 2rem" v-html="tt('labels_note')"></p>
    <div class="field-row">
      <div v-for="option in options">
        <cdx-field :label-icon="cdxIconTextSummary">
          <cdx-text-area name="templates" v-model="state[`labels_${option}`]" />
          <template #label>{{ tt("labels") }}</template>
          <template #description>
            {{ tt(`labels_${option}`) }}
            <br />
            {{ tt(`ph_labels`) }}
          </template>
          <cdx-checkbox
            v-model="state[`cb_labels_${option}_l`]"
            :name="`cb_labels_${option}_l`"
            :inline="true"
            style="margin-top: 0.5rem"
          >
            {{ tt("labels") }}
          </cdx-checkbox>
          <cdx-checkbox
            v-model="state[`cb_labels_${option}_a`]"
            :name="`cb_labels_${option}_a`"
            :inline="true"
            style="margin-top: 0.5rem"
          >
            {{ tt("aliases") }}
          </cdx-checkbox>
          <cdx-checkbox
            v-model="state[`cb_labels_${option}_d`]"
            :name="`cb_labels_${option}_d`"
            :inline="true"
            style="margin-top: 0.5rem"
          >
            {{ tt("descriptions") }}
          </cdx-checkbox>
          <cdx-text-input
            :name="`langs_labels_${option}`"
            v-model="state[`langs_labels_${option}`]"
            :placeholder="tt('ph_langs_labels')"
          />
        </cdx-field>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import {
  CdxCheckbox,
  CdxField,
  CdxRadio,
  CdxTextArea,
  CdxTextInput,
} from "@wikimedia/codex";
import tt from "../i18n/tt";
import { useState } from "../useState";
import { cdxIconLink, cdxIconTextSummary } from "@wikimedia/codex-icons";

const state = useState();
const wikidataItemOptions = ["no", "any", "with", "without"] as const;
const wpiuOptions = ["any", "all", "none"] as const;
const options = ["yes", "any", "no"] as const;
</script>
