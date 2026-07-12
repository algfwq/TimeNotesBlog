import { useEffect, useMemo, useState } from 'react';
import { VChart } from '@visactor/react-vchart';
import { VChart as VChartCore } from '@visactor/vchart';
import worldGeo from '../data/world-countries.geo.json';
import { useTheme } from '../theme';

type CountryCount = { country: string; count: number };
type Location = {
  country: string;
  region: string;
  city: string;
  lat: number;
  lng: number;
  count: number;
};

let mapRegistered = false;

function ensureWorldMap() {
  if (mapRegistered) return;
  VChartCore.registerMap('world', worldGeo as never, { type: 'geojson', rewind: true });
  mapRegistered = true;
}

/** Map common GeoIP country labels onto the simplified world GeoJSON names. */
const COUNTRY_ALIASES: Record<string, string> = {
  'United States': 'United States of America',
  'United States of America': 'United States of America',
  USA: 'United States of America',
  US: 'United States of America',
  'United Kingdom': 'United Kingdom',
  UK: 'United Kingdom',
  'South Korea': 'South Korea',
  'Korea, Republic of': 'South Korea',
  'North Korea': 'North Korea',
  Russia: 'Russia',
  'Russian Federation': 'Russia',
  Vietnam: 'Vietnam',
  'Viet Nam': 'Vietnam',
  Iran: 'Iran',
  Syria: 'Syria',
  Tanzania: 'United Republic of Tanzania',
  'Czech Republic': 'Czech Republic',
  Czechia: 'Czech Republic',
  'Congo (Kinshasa)': 'Democratic Republic of the Congo',
  'Congo (Brazzaville)': 'Republic of the Congo',
  'Cote d\'Ivoire': 'Ivory Coast',
  "Côte d'Ivoire": 'Ivory Coast',
  Burma: 'Myanmar',
  'Taiwan, Province of China': 'Taiwan',
  Taiwan: 'Taiwan',
  'Hong Kong': 'Hong Kong',
  Macao: 'Macao',
  Palestine: 'Palestine',
  Bolivia: 'Bolivia',
  Venezuela: 'Venezuela',
  Laos: 'Laos',
  Brunei: 'Brunei',
  Moldova: 'Moldova',
  Macedonia: 'North Macedonia',
  'North Macedonia': 'North Macedonia',
};

function normalizeCountry(name: string): string {
  const n = name.trim();
  if (!n) return '';
  return COUNTRY_ALIASES[n] || n;
}

export function WorldMapChart({
  countries = [],
  locations = [],
}: {
  countries?: CountryCount[];
  locations?: Location[];
}) {
  const { mode } = useTheme();
  const [ready, setReady] = useState(mapRegistered);

  useEffect(() => {
    try {
      ensureWorldMap();
      setReady(true);
    } catch (e) {
      console.error('register world map failed', e);
    }
  }, []);

  const values = useMemo(() => {
    const map = new Map<string, number>();
    for (const c of countries) {
      const key = normalizeCountry(c.country);
      if (!key) continue;
      map.set(key, (map.get(key) || 0) + Number(c.count || 0));
    }
    // Fallback: aggregate from locations if countries empty.
    if (map.size === 0) {
      for (const loc of locations) {
        const key = normalizeCountry(loc.country);
        if (!key) continue;
        map.set(key, (map.get(key) || 0) + Number(loc.count || 0));
      }
    }
    return [...map.entries()].map(([name, value]) => ({ name, value }));
  }, [countries, locations]);

  const tooltipRows = useMemo(() => {
    const byCountry = new Map<string, Location[]>();
    for (const loc of locations) {
      const key = normalizeCountry(loc.country);
      if (!key) continue;
      const list = byCountry.get(key) || [];
      list.push(loc);
      byCountry.set(key, list);
    }
    return byCountry;
  }, [locations]);

  const isDark = mode === 'dark';

  const spec = useMemo(() => {
    const max = values.reduce((m, v) => Math.max(m, v.value), 0) || 1;
    return {
      type: 'map',
      map: 'world',
      nameField: 'name',
      valueField: 'value',
      title: {
        text: '访客地理分布',
        textStyle: { fill: isDark ? '#f4f1ea' : '#1f2430', fontSize: 14, fontWeight: 600 },
      },
      background: 'transparent',
      data: {
        values: values.length ? values : [{ name: '__none__', value: 0 }],
      },
      region: [
        {
          roam: true,
          coordinate: 'geo',
          zoomLimit: { min: 0.8, max: 8 },
        },
      ],
      color: {
        type: 'linear',
        domain: [0, max],
        range: isDark
          ? ['#1e293b', '#0ea5e9', '#38bdf8', '#e0f2fe']
          : ['#e2e8f0', '#93c5fd', '#3b82f6', '#1d4ed8'],
      },
      area: {
        style: {
          fillOpacity: 0.92,
          stroke: isDark ? 'rgba(255,255,255,0.12)' : 'rgba(15,23,42,0.12)',
          lineWidth: 0.5,
        },
      },
      tooltip: {
        mark: {
          content: [
            {
              key: '国家/地区',
              value: (datum: { name?: string }) => datum?.name || '-',
            },
            {
              key: '访问次数',
              value: (datum: { value?: number; name?: string }) => {
                const n = Number(datum?.value || 0);
                const rows = tooltipRows.get(String(datum?.name || '')) || [];
                if (!rows.length) return String(n);
                const top = rows
                  .slice()
                  .sort((a, b) => b.count - a.count)
                  .slice(0, 3)
                  .map((r) => [r.city, r.region].filter(Boolean).join(', ') || r.country)
                  .join(' / ');
                return top ? `${n}（${top}）` : String(n);
              },
            },
          ],
        },
      },
      legends: {
        visible: true,
        orient: 'bottom',
        type: 'color',
        field: 'value',
      },
    };
  }, [values, isDark, tooltipRows]);

  if (!ready) {
    return <div className="muted" style={{ padding: 24 }}>地图加载中…</div>;
  }

  return <VChart spec={spec as never} style={{ width: '100%', height: '100%', minHeight: 300 }} />;
}
