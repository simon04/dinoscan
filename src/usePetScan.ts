import { useFetch } from "@vueuse/core";
import { computed, toRaw } from "vue";
import { useState } from "./useState";

const state = useState();

export const { execute, isFetching, isFinished, data } = useFetch(
  computed(() => {
    const params = new URLSearchParams();
    Object.entries(toRaw(state)).forEach(
      ([key, value]) => value && params.set(key, String(value)),
    );
    return "https://petscan.wmcloud.org/?" + params;
  }),
  { immediate: false },
).json<PetScan>();

export const results = computed<Result[]>(
  () => data.value?.["*"]?.[0]?.a?.["*"] ?? [],
);

export const query = computed<URL>(() => new URL(data.value?.a.query ?? ""));

export const error = computed<string | undefined>(() => data.value?.error);

export function usePetScan() {
  return { execute, isFetching, isFinished, results, query, error };
}

interface PetScan {
  "*": [
    {
      a: {
        "*": Result[];
        type: string;
      };
      n: string;
    },
  ];
  a: {
    query: string;
    querytime_sec: number;
  };
  n: string;
  error?: string;
}

export interface Result {
  id: number;
  len: number;
  metadata: {
    wikidata: string;
  };
  n: string;
  namespace: number;
  nstext: string;
  q: string;
  title: string;
  touched: string;
}
