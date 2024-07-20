import { useFetch } from "@vueuse/core";
import { useState } from "./useState";
import { computed } from "vue";

const state = useState();

const { data } = await useFetch(
  computed(
    () =>
      `https://${state.language}.${state.project}.org/w/api.php?origin=*&action=query&meta=siteinfo&siprop=namespaces&format=json&formatversion=2`,
  ),
  { refetch: true },
).json<{ query: Query }>();

const namespaces = computed((): Namespace[] =>
  Object.values(data.value?.query.namespaces ?? {}),
);

export function useNamespaces() {
  return { namespaces };
}

export interface Welcome {
  batchcomplete: boolean;
  query: Query;
}

export interface Query {
  namespaces: Record<string, Namespace>;
}

export interface Namespace {
  id: number;
  case: "first-letter";
  name: string;
  subpages: boolean;
  canonical?: string;
  content: boolean;
  nonincludable: boolean;
  namespaceprotection?: string;
}
