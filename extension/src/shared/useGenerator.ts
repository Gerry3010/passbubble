// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

import { useEffect, useState } from 'react';
import { generatePassword, validateGeneratorOptions, type GeneratorOptions } from '@passbubble/shared-ts';
import { readGeneratorDefaults } from './generator-defaults.js';

/** The stored generator defaults, or null while they load. */
export function useGeneratorDefaults(): GeneratorOptions | null {
  const [defaults, setDefaults] = useState<GeneratorOptions | null>(null);
  useEffect(() => {
    let alive = true;
    void readGeneratorDefaults().then((d) => {
      if (alive) setDefaults(d);
    });
    return () => {
      alive = false;
    };
  }, []);
  return defaults;
}

interface Generated {
  password: string;
  error: string | null;
}

function run(options: GeneratorOptions): Generated {
  const error = validateGeneratorOptions(options);
  return error ? { password: '', error } : { password: generatePassword(options), error: null };
}

/** Generator state for one panel or dialog. The options start from `initial`
 * and live only here; every change generates a new password. An
 * `initialPassword` is shown first instead of a fresh one (the fill box
 * re-shows the password already used on the page). */
export function useGenerator(initial: GeneratorOptions, initialPassword?: string) {
  const [options, setOptionsState] = useState(initial);
  const [result, setResult] = useState<Generated>(() =>
    initialPassword ? { password: initialPassword, error: null } : run(initial),
  );

  function setOptions(next: GeneratorOptions) {
    setOptionsState(next);
    setResult(run(next));
  }

  function regenerate() {
    setResult(run(options));
  }

  return { options, setOptions, password: result.password, error: result.error, regenerate };
}
