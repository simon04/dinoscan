import { useFetch } from "@vueuse/core";
import { computed } from "vue";
import { useState } from "./useState";

const state = useState();

export const { execute, isFetching, isFinished, data } = useFetch(
  computed(
    () =>
      "https://petscan.wmcloud.org/?" +
      new URLSearchParams(state as unknown as Record<string, string>),
  ),
  { immediate: false },
).json<PetScan>();

export const results = computed<Result[]>(
  () => data.value?.["*"][0].a["*"] ?? [],
);

export function usePetScan() {
  return { execute, isFetching, isFinished, results };
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
