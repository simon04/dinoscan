<template>
  <section>
    <cdx-field>
      <template #label>{{ tt("sparql") }}</template>
      <template #description>{{ tt("sparql_note") }}</template>
      <cdx-text-area v-model="state.sparql" name="sparql" />
    </cdx-field>

    <cdx-field style="margin-top: 2rem">
      <template #label>{{ tt("manual_list") }}</template>
      <template #description>{{ tt("manual_list_note") }}</template>
      <cdx-text-area v-model="state.manual_list" name="manual_list" />
    </cdx-field>
    <cdx-field>
      <template #label>
        {{ tt("manual_list") }}: {{ tt("manual_list_wiki") }}
      </template>
      <template #description>{{ tt("manual_list_wiki_note") }}</template>
      <cdx-text-input
        v-model="state.manual_list_wiki"
        name="manual_list_wiki"
      />
    </cdx-field>

    <cdx-field style="margin-top: 2rem">
      <template #label>{{ tt("pagepile") }}</template>
      <template #description>{{ tt("pagepile_note") }}</template>
      <cdx-text-input
        v-model="state.pagepile"
        name="pagepile"
        input-type="number"
      />
    </cdx-field>

    <div class="field-row" style="margin-top: 2rem">
      <div style="flex: 2 2">
        <cdx-field>
          <template #label>{{ tt("search") }}</template>
          <template #description>{{ tt("search_note") }}</template>
          <cdx-text-input v-model="state.search_query" name="search_query" />
        </cdx-field>
      </div>
      <div>
        <cdx-field>
          <template #label>
            {{ tt("search") }}: {{ tt("manual_list_wiki") }}
          </template>
          <template #description>{{ tt("search_wiki_note") }}</template>
          <cdx-text-input v-model="state.search_wiki" name="search_wiki" />
        </cdx-field>
      </div>
      <div>
        <cdx-field>
          <template #label>
            {{ tt("search") }}: {{ tt("max_results") }}
          </template>
          <template #description>&nbsp;</template>
          <cdx-text-input
            v-model="state.search_max_results"
            name="search_max_results"
            input-type="number"
          />
        </cdx-field>
      </div>
    </div>

    <cdx-field style="margin-top: 2rem">
      <template #label>{{ tt("wikidata") }}</template>
      <template #description>{{ tt("wikidata_source_note") }}</template>
      <template #help-text>{{ tt("wikidata_source_ph") }}</template>
      <cdx-text-input
        v-model="state.wikidata_source_sites"
        name="wikidata_source_sites"
      />
    </cdx-field>

    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("namespaces") }}</template>
      <cdx-radio
        v-for="option of namespaceOptions"
        v-model="state.namespace_conversion"
        name="namespace_conversion"
        :input-value="option"
        :inline="true"
      >
        {{ tt(option) }}
      </cdx-radio>
    </cdx-field>

    <!-- TODO subpages -->

    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("common_wiki") }}</template>
      <cdx-radio
        v-for="option of wikiOptions"
        v-model="state.common_wiki"
        name="common_wiki"
        :input-value="option"
        :inline="true"
      >
        <cdx-text-input
          v-if="option === 'other'"
          v-model="state.common_wiki_other"
          name="common_wiki_other"
          :placeholder="tt(`common_wiki_${option}`)"
        />
        <span v-else>{{ tt(`common_wiki_${option}`) }}</span>
      </cdx-radio>
    </cdx-field>

    <cdx-field style="margin-top: 2rem">
      <template #label>{{ tt("source_combination") }}</template>
      <template #description>{{ tt("source_combination_hint") }}</template>
      <template #help-text>
        <span v-html="tt('source_combination_description')"></span>
      </template>
      <cdx-text-input
        v-model="state.source_combination"
        name="source_combination"
      />
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
const namespaceOptions = [
  "namespace_conversion_keep",
  "namespace_conversion_topic",
  "namespace_conversion_talk",
] as const;
const wikiOptions = [
  "auto",
  "cats",
  "pagepile",
  "manual",
  "wikidata",
  "other",
] as const;
</script>
