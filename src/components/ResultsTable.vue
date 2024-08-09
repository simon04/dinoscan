<template>
  <cdx-message v-if="error" type="error">
    {{ error }}
  </cdx-message>
  <cdx-table
    v-else
    v-model:sort="sort"
    class="cdx-docs-table-with-sort"
    :caption="tt('results')"
    :columns="sortedResults.length ? columns : undefined"
    :data="sortedResults"
    @update:sort="sort = $event"
  >
    <template #empty-state>{{ tt("num_results").replace("$1", "0") }}</template>
    <template #item-title="{ item }">
      <a
        :href="`https://${query.language}.${query.project}.org/wiki/${item}`"
        target="_blank"
        rel="external noopener"
      >
        {{ item.replace(/_/g, " ") }}
      </a>
    </template>
    <template #item-namespace="{ item }">
      {{ tt(`namespace_${item}` as unknown as "namespace_0") }}
    </template>
    <template #item-touched="{ item }">
      <time style="white-space: nowrap">
        {{ item.slice(0, 4) }}-{{ item.slice(4, 6) }}-{{ item.slice(6, 8) }}
      </time>
      {{ " " }}
      <time style="white-space: nowrap">
        {{ item.slice(8, 10) }}:{{ item.slice(10, 12) }}:{{
          item.slice(12, 14)
        }}
      </time>
    </template>
    <template #item-q="{ item }">
      <a
        :href="`https://www.wikidata.org/wiki/${item}`"
        target="_blank"
        rel="external noopener"
      >
        {{ item }}
      </a>
    </template>
  </cdx-table>
</template>

<script setup lang="ts">
import {
  CdxMessage,
  CdxTable,
  TableColumn,
  TableSort,
  TableSortOption,
} from "@wikimedia/codex";
import { computed, ref } from "vue";
import tt from "../i18n/tt";
import { Result, usePetScan } from "../usePetScan";

const { results, query, error } = usePetScan();

const columns = computed(
  () =>
    [
      {
        id: "title",
        label: tt("h_title"),
        allowSort: true,
      },
      {
        id: "id",
        label: tt("h_id"),
        allowSort: true,
        textAlign: "number",
      },
      {
        id: "namespace",
        label: tt("h_namespace"),
        allowSort: true,
      },
      {
        id: "len",
        label: tt("h_len"),
        allowSort: true,
        textAlign: "number",
      },
      {
        id: "touched",
        label: tt("h_touched"),
        allowSort: true,
        textAlign: "number",
      },
      {
        id: "q",
        label: tt("wikidata"),
        allowSort: true,
        textAlign: "number",
      },
    ] satisfies (TableColumn & {
      id: keyof Result;
    })[],
);

const sort = ref<TableSort<keyof Result>>({ id: "asc" });

const sortedResults = computed(() => {
  const [[sortKey, sortOrder]] = Object.entries(sort.value) as [
    [keyof Result, TableSortOption],
  ];
  const multiplier = sortOrder === "asc" ? 1 : -1;
  return results.value.sort(
    (a, b) =>
      multiplier *
      (typeof a[sortKey] === "number" && typeof b[sortKey] === "number"
        ? a[sortKey] - b[sortKey]
        : String(a[sortKey]).localeCompare(String(b[sortKey]))),
  );
});
</script>
