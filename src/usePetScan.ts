import { useFetch } from "@vueuse/core";
import { computed } from "vue";
import { useState } from "./useState";

const state = useState();

const url = computed(() => {
  const params = new URLSearchParams();
  Object.entries(JSON.parse(JSON.stringify(state))).forEach(
    ([key, value]) => value && params.set(key, String(value)),
  );
  return "https://petscan.wmcloud.org/?" + params;
});

const { execute, isFetching, isFinished, data } = useFetch(url, {
  immediate: false,
}).json<PetScan>();

const results = computed<Result[]>(
  () => data.value?.["*"]?.[0]?.a?.["*"] ?? [],
);

const query = computed<typeof state>(
  () =>
    Object.fromEntries(
      new URL(data.value?.a.query ?? "").searchParams.entries(),
    ) as unknown as typeof state,
);

const wiki = computed(
  () => `https://${query.value.language}.${query.value.project}.org/wiki`,
);

const error = computed<string | undefined>(() => data.value?.error);

export function usePetScan() {
  return { url, wiki, execute, isFetching, isFinished, results, query, error };
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
    coordinates?: string;
    defaultsort?: string;
    disambiguation: boolean;
    image?: string;
    wikidata: string;
  };
  n: string;
  namespace: number;
  nstext: string;
  q: string;
  title: string;
  touched: string;
}
