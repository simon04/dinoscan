<template>
  <section>
    <div class="field-row">
      <div>
        <cdx-field :is-fieldset="true">
          <template #label>{{ tt("namespaces") }}</template>
          <cdx-checkbox
            v-for="ns in namespaces"
            :key="ns.id + ns.name"
            v-model="state[`ns[${ns.id}]`]"
            :inline="true"
          >
            {{ ns.id === 0 ? tt("namespace_0") : ns.name }}
          </cdx-checkbox>
        </cdx-field>
      </div>
    </div>

    <div class="field-row">
      <div>
        <cdx-field>
          <cdx-text-input
            name="larger"
            v-model="state.larger"
            input-type="number"
          />
          <template #label>{{ tt("size") }}</template>
          <template #description>
            {{ tt("larger") }} {{ tt("size_explanation") }}
          </template>
        </cdx-field>
      </div>
      <div>
        <cdx-field>
          <cdx-text-input
            name="smaller"
            v-model="state.smaller"
            input-type="number"
          />
          <template #label>{{ tt("size") }}</template>
          <template #description>
            {{ tt("smaller") }} {{ tt("size_explanation") }}
          </template>
        </cdx-field>
      </div>
      <div>
        <cdx-field>
          <cdx-text-input
            name="since_rev0"
            v-model="state.since_rev0"
            input-type="number"
          />
          <template #label>{{ tt("size") }}</template>
          <template #description>
            {{ tt("since_rev0") }}
          </template>
          <template #help-text>
            {{ tt("since_rev0_explanation") }}
          </template>
        </cdx-field>
      </div>
    </div>

    <div class="field-row">
      <div>
        <cdx-field>
          <cdx-text-input
            name="larger"
            v-model="state.minlinks"
            input-type="number"
          />
          <template #label>{{ tt("link_number") }}</template>
          <template #description>{{ tt("minlinks") }}</template>
        </cdx-field>
      </div>
      <div>
        <cdx-field>
          <cdx-text-input
            name="smaller"
            v-model="state.maxlinks"
            input-type="number"
          />
          <template #label>{{ tt("link_number") }}</template>
          <template #description>{{ tt("maxlinks") }}</template>
        </cdx-field>
      </div>
    </div>

    <div class="field-row">
      <div>
        <cdx-field>
          <cdx-text-input
            name="larger"
            v-model="state.before"
            input-type="number"
          />
          <template #label>{{ tt("edits") }}</template>
          <template #description>{{ tt("before") }}</template>
          <template #help-text>{{ tt("date_format") }}</template>
        </cdx-field>
      </div>
      <div>
        <cdx-field>
          <cdx-text-input
            name="smaller"
            v-model="state.after"
            input-type="number"
          />
          <template #label>{{ tt("edits") }}</template>
          <template #description>{{ tt("after") }}</template>
          <template #help-text>{{ tt("date_format") }}</template>
        </cdx-field>
      </div>
    </div>

    <div class="field-row">
      <div>
        <cdx-field :is-fieldset="true" v-for="show_redirects in tertiaryFields">
          <template #label>{{ tt(show_redirects) }}</template>
          <cdx-radio
            v-for="option of tertiaryOptions"
            v-model="state[show_redirects]"
            :name="show_redirects"
            :input-value="option"
            :inline="true"
          >
            {{ tt(`edits_${option}`) }}
          </cdx-radio>
        </cdx-field>
      </div>

      <div>
        <cdx-field :is-fieldset="true" v-for="edit in tertiaryEditFields">
          <template #label>
            {{ tt("edits") }}: {{ tt(`edit_${edit}`) }}
          </template>
          <cdx-radio
            v-for="option of tertiaryOptions"
            v-model="state[`edits[${edit}]`]"
            :name="edit"
            :input-value="option"
            :inline="true"
          >
            {{ tt(`edits_${option}`) }}
          </cdx-radio>
        </cdx-field>
      </div>
    </div>

    <cdx-field :is-fieldset="true">
      <template #label>{{ tt("page_image") }}</template>
      <cdx-radio
        v-for="option of pageImageOptions"
        v-model="state.page_image"
        name="page_image"
        :input-value="option"
        :inline="true"
      >
        {{ tt(`page_image_${option}`) }}
      </cdx-radio>
    </cdx-field>

    <!-- TODO: ORES -->
  </section>
</template>

<script setup lang="ts">
import {
  CdxField,
  CdxRadio,
  CdxTextInput,
  CdxCheckbox,
} from "@wikimedia/codex";
import tt from "../i18n/tt";
import { useNamespaces } from "../useNamespaces";
import { useState } from "../useState";

const state = useState();
const { namespaces } = useNamespaces();
const pageImageOptions = ["any", "yes", "free", "nonfree", "no"] as const;
const tertiaryOptions = ["both", "yes", "no"] as const;
const tertiaryFields = [
  "show_redirects",
  "show_soft_redirects",
  "show_disambiguation_pages",
] as const;
const tertiaryEditFields = ["bots", "anons", "flagged"] as const;
</script>

<style lang="scss">
.field-row {
  margin-bottom: 1rem;
  display: flex;
  gap: 1rem;
}
.field-row > div {
  flex: 1 1;
}
</style>
