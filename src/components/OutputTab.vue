<template>
  <section>
    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("format") }}</template>
      <cdx-radio
        v-for="[option, label] of formatOptions"
        v-model="state.format"
        name="format"
        :input-value="option"
        :inline="true"
      >
        {{ label }}
      </cdx-radio>
    </cdx-field>

    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("output_compatability") }}</template>
      <cdx-radio
        v-for="[option, label] of compatibilityOptions"
        v-model="state.output_compatability"
        name="output_compatability"
        :input-value="option"
        :inline="true"
      >
        {{ label }}
      </cdx-radio>
      <cdx-checkbox v-model="state.sparse" name="sparse" :inline="true">
        {{ tt("sparse") }}
      </cdx-checkbox>
      <cdx-checkbox
        v-model="state['json-pretty']"
        name="pretty_json"
        :inline="true"
      >
        {{ tt("pretty_json") }}
      </cdx-checkbox>
    </cdx-field>

    <!-- TODO: sort -->
    <!-- TODO: sort order -->

    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("files") }}</template>
      <cdx-checkbox
        v-for="option of filesOptions"
        v-model="state[option]"
        :name="option"
        :input-value="option"
        :inline="true"
      >
        {{ tt(option) }}
      </cdx-checkbox>
    </cdx-field>

    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("add_metadata") }}</template>
      <cdx-checkbox
        v-for="option of metadataOptions"
        v-model="state[option]"
        :name="option"
        :input-value="option"
        :inline="true"
      >
        {{ tt(option) }}
      </cdx-checkbox>
    </cdx-field>

    <cdx-field>
      <template #label>{{ tt("regexp_filter") }}</template>
      <template #description>{{ tt("regexp_filter_note") }}</template>
      <template #help-text>{{ tt("regexp_filter_rules") }}</template>
      <cdx-text-input v-model="state.rxp_filter" name="rxp_filter" />
    </cdx-field>

    <cdx-field>
      <template #label>{{ tt("search_filter") }}</template>
      <template #description>{{ tt("search_filter_note") }}</template>
      <template #help-text>{{ tt("search_filter_rules") }}</template>
      <cdx-text-input v-model="state.search_filter" name="search_filter" />
    </cdx-field>

    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("redlinks") }}</template>
      <cdx-checkbox
        v-for="option of redlinkOptions"
        v-model="state[option]"
        :name="option"
        :input-value="option"
        :inline="true"
      >
        {{ tt(option) }}
      </cdx-checkbox>
    </cdx-field>

    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("min_redlink_count") }}</template>
      <cdx-text-input
        v-model="state.min_redlink_count"
        name="min_redlink_count"
        input-type="number"
      />
    </cdx-field>

    <cdx-field>
      <template #label>{{ tt("limit") }}</template>
      <template #description>{{ tt("ph_limit") }}</template>
      <cdx-text-input
        v-model="state.output_limit"
        name="output_limit"
        input-type="number"
      />
    </cdx-field>
  </section>
</template>

<script setup lang="ts">
import {
  CdxCheckbox,
  CdxField,
  CdxTextInput,
  CdxRadio,
} from "@wikimedia/codex";
import tt from "../i18n/tt";
import { useState } from "../useState";

const state = useState();
const formatOptions = [
  ["html", "HTML"],
  ["csv", "CSV"],
  ["tsv", "TSV"],
  ["wiki", "Wiki"],
  ["json", "JSON"],
  ["pagepile", "PagePile"],
  ["kml", "KML"],
  ["plain", tt("plain_text")],
] as const;
const compatibilityOptions = [
  ["catscan", tt("catscan")],
  ["quick-intersection", tt("quick_intersection")],
] as const;
const filesOptions = [
  "ext_image_data",
  "file_usage_data",
  "file_usage_data_ns0",
  "thumbnails_in_wiki_output",
] as const;
const metadataOptions = [
  "add_image",
  "add_coordinates",
  "add_defaultsort",
  "add_disambiguation",
] as const;
const redlinkOptions = [
  "show_redlinks",
  "remove_template_redlinks",
  "article_redlinks_only",
] as const;
</script>
