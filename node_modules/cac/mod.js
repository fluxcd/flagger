import { EventEmitter } from 'events';

function toArr(any) {
	return any == null ? [] : Array.isArray(any) ? any : [any];
}

function toVal(out, key, val, opts) {
	var x, old=out[key], nxt=(
		!!~opts.string.indexOf(key) ? (val == null || val === true ? '' : String(val))
		: typeof val === 'boolean' ? val
		: !!~opts.boolean.indexOf(key) ? (val === 'false' ? false : val === 'true' || (out._.push((x = +val,x * 0 === 0) ? x : val),!!val))
		: (x = +val,x * 0 === 0) ? x : val
	);
	out[key] = old == null ? nxt : (Array.isArray(old) ? old.concat(nxt) : [old, nxt]);
}

var lib = function (args, opts) {
	args = args || [];
	opts = opts || {};

	var k, arr, arg, name, val, out={ _:[] };
	var i=0, j=0, idx=0, len=args.length;

	const alibi = opts.alias !== void 0;
	const strict = opts.unknown !== void 0;
	const defaults = opts.default !== void 0;

	opts.alias = opts.alias || {};
	opts.string = toArr(opts.string);
	opts.boolean = toArr(opts.boolean);

	if (alibi) {
		for (k in opts.alias) {
			arr = opts.alias[k] = toArr(opts.alias[k]);
			for (i=0; i < arr.length; i++) {
				(opts.alias[arr[i]] = arr.concat(k)).splice(i, 1);
			}
		}
	}

	opts.boolean.forEach(key => {
		opts.boolean = opts.boolean.concat(opts.alias[key] = opts.alias[key] || []);
	});

	opts.string.forEach(key => {
		opts.string = opts.string.concat(opts.alias[key] = opts.alias[key] || []);
	});

	if (defaults) {
		for (k in opts.default) {
			opts.alias[k] = opts.alias[k] || [];
			(opts[typeof opts.default[k]] || []).push(k);
		}
	}

	const keys = strict ? Object.keys(opts.alias) : [];

	for (i=0; i < len; i++) {
		arg = args[i];

		if (arg === '--') {
			out._ = out._.concat(args.slice(++i));
			break;
		}

		for (j=0; j < arg.length; j++) {
			if (arg.charCodeAt(j) !== 45) break; // "-"
		}

		if (j === 0) {
			out._.push(arg);
		} else if (arg.substring(j, j + 3) === 'no-') {
			name = arg.substring(j + 3);
			if (strict && !~keys.indexOf(name)) {
				return opts.unknown(arg);
			}
			out[name] = false;
		} else {
			for (idx=j+1; idx < arg.length; idx++) {
				if (arg.charCodeAt(idx) === 61) break; // "="
			}

			name = arg.substring(j, idx);
			val = arg.substring(++idx) || (i+1 === len || (''+args[i+1]).charCodeAt(0) === 45 || args[++i]);
			arr = (j === 2 ? [name] : name);

			for (idx=0; idx < arr.length; idx++) {
				name = arr[idx];
				if (strict && !~keys.indexOf(name)) return opts.unknown('-'.repeat(j) + name);
				toVal(out, name, (idx + 1 < arr.length) || val, opts);
			}
		}
	}

	if (defaults) {
		for (k in opts.default) {
			if (out[k] === void 0) {
				out[k] = opts.default[k];
			}
		}
	}

	if (alibi) {
		for (k in out) {
			arr = opts.alias[k] || [];
			while (arr.length > 0) {
				out[arr.shift()] = out[k];
			}
		}
	}

	return out;
};

const removeBrackets = (v) => v.replace(/[<[].+/, '').trim();
const findAllBrackets = (v) => {
    const ANGLED_BRACKET_RE_GLOBAL = /<([^>]+)>/g;
    const SQUARE_BRACKET_RE_GLOBAL = /\[([^\]]+)\]/g;
    const res = [];
    const parse = (match) => {
        let variadic = false;
        let value = match[1];
        if (value.startsWith('...')) {
            value = value.slice(3);
            variadic = true;
        }
        return {
            required: match[0].startsWith('<'),
            value,
            variadic
        };
    };
    let angledMatch;
    while ((angledMatch = ANGLED_BRACKET_RE_GLOBAL.exec(v))) {
        res.push(parse(angledMatch));
    }
    let squareMatch;
    while ((squareMatch = SQUARE_BRACKET_RE_GLOBAL.exec(v))) {
        res.push(parse(squareMatch));
    }
    return res;
};
const getMriOptions = (options) => {
    const result = { alias: {}, boolean: [] };
    for (const [index, option] of options.entries()) {
        // We do not set default values in mri options
        // Since its type (typeof) will be used to cast parsed arguments.
        // Which mean `--foo foo` will be parsed as `{foo: true}` if we have `{default:{foo: true}}`
        // Set alias
        if (option.names.length > 1) {
            result.alias[option.names[0]] = option.names.slice(1);
        }
        // Set boolean
        if (option.isBoolean) {
            if (option.negated) {
                // For negated option
                // We only set it to `boolean` type when there's no string-type option with the same name
                const hasStringTypeOption = options.some((o, i) => {
                    return (i !== index &&
                        o.names.some(name => option.names.includes(name)) &&
                        typeof o.required === 'boolean');
                });
                if (!hasStringTypeOption) {
                    result.boolean.push(option.names[0]);
                }
            }
            else {
                result.boolean.push(option.names[0]);
            }
        }
    }
    return result;
};
const findLongest = (arr) => {
    return arr.sort((a, b) => {
        return a.length > b.length ? -1 : 1;
    })[0];
};
const padRight = (str, length) => {
    return str.length >= length ? str : `${str}${' '.repeat(length - str.length)}`;
};
const camelcase = (input) => {
    return input.replace(/([a-z])-([a-z])/g, (_, p1, p2) => {
        return p1 + p2.toUpperCase();
    });
};
const setDotProp = (obj, keys, val) => {
    let i = 0;
    let length = keys.length;
    let t = obj;
    let x;
    for (; i < length; ++i) {
        x = t[keys[i]];
        t = t[keys[i]] =
            i === length - 1
                ? val
                : x != null
                    ? x
                    : !!~keys[i + 1].indexOf('.') || !(+keys[i + 1] > -1)
                        ? {}
                        : [];
    }
};
const setByType = (obj, transforms) => {
    for (const key of Object.keys(transforms)) {
        const transform = transforms[key];
        if (transform.shouldTransform) {
            obj[key] = Array.prototype.concat.call([], obj[key]);
            if (typeof transform.transformFunction === 'function') {
                obj[key] = obj[key].map(transform.transformFunction);
            }
        }
    }
};
const getFileName = (input) => {
    const m = /([^\\\/]+)$/.exec(input);
    return m ? m[1] : '';
};

class Option {
    constructor(rawName, description, config) {
        this.rawName = rawName;
        this.description = description;
        this.config = Object.assign({}, config);
        // You may use cli.option('--env.* [value]', 'desc') to denote a dot-nested option
        rawName = rawName.replace(/\.\*/g, '');
        this.negated = false;
        this.names = removeBrackets(rawName)
            .split(',')
            .map((v) => {
            let name = v.trim().replace(/^-{1,2}/, '');
            if (name.startsWith('no-')) {
                this.negated = true;
                name = name.replace(/^no-/, '');
            }
            return name;
        })
            .sort((a, b) => (a.length > b.length ? 1 : -1)); // Sort names
        // Use the longese name (last one) as actual option name
        this.name = this.names[this.names.length - 1];
        if (this.negated) {
            this.config.default = true;
        }
        if (rawName.includes('<')) {
            this.required = true;
        }
        else if (rawName.includes('[')) {
            this.required = false;
        }
        else {
            // No arg needed, it's boolean flag
            this.isBoolean = true;
        }
    }
}

const deno = typeof window !== 'undefined' && window.Deno;
const exit = (code) => {
    return deno ? Deno.exit(code) : process.exit(code);
};
const processArgs = deno ? ['deno'].concat(Deno.args) : process.argv;
const platformInfo = deno
    ? `${Deno.platform.os}-${Deno.platform.arch} deno-${Deno.version.deno}`
    : `${process.platform}-${process.arch} node-${process.version}`;

class Command {
    constructor(rawName, description, config = {}, cli) {
        this.rawName = rawName;
        this.description = description;
        this.config = config;
        this.cli = cli;
        this.options = [];
        this.aliasNames = [];
        this.name = removeBrackets(rawName);
        this.args = findAllBrackets(rawName);
        this.examples = [];
    }
    usage(text) {
        this.usageText = text;
        return this;
    }
    allowUnknownOptions() {
        this.config.allowUnknownOptions = true;
        return this;
    }
    ignoreOptionDefaultValue() {
        this.config.ignoreOptionDefaultValue = true;
        return this;
    }
    version(version, customFlags = '-v, --version') {
        this.versionNumber = version;
        this.option(customFlags, 'Display version number');
        return this;
    }
    example(example) {
        this.examples.push(example);
        return this;
    }
    /**
     * Add a option for this command
     * @param rawName Raw option name(s)
     * @param description Option description
     * @param config Option config
     */
    option(rawName, description, config) {
        const option = new Option(rawName, description, config);
        this.options.push(option);
        return this;
    }
    alias(name) {
        this.aliasNames.push(name);
        return this;
    }
    action(callback) {
        this.commandAction = callback;
        return this;
    }
    /**
     * Check if a command name is matched by this command
     * @param name Command name
     */
    isMatched(name) {
        return this.name === name || this.aliasNames.includes(name);
    }
    get isDefaultCommand() {
        return this.name === '' || this.aliasNames.includes('!');
    }
    get isGlobalCommand() {
        return this instanceof GlobalCommand;
    }
    /**
     * Check if an option is registered in this command
     * @param name Option name
     */
    hasOption(name) {
        name = name.split('.')[0];
        return this.options.find(option => {
            return option.names.includes(name);
        });
    }
    outputHelp() {
        const { name, commands } = this.cli;
        const { versionNumber, options: globalOptions, helpCallback } = this.cli.globalCommand;
        const sections = [
            {
                body: `${name}${versionNumber ? ` v${versionNumber}` : ''}`
            }
        ];
        sections.push({
            title: 'Usage',
            body: `  $ ${name} ${this.usageText || this.rawName}`
        });
        const showCommands = (this.isGlobalCommand || this.isDefaultCommand) && commands.length > 0;
        if (showCommands) {
            const longestCommandName = findLongest(commands.map(command => command.rawName));
            sections.push({
                title: 'Commands',
                body: commands
                    .map(command => {
                    return `  ${padRight(command.rawName, longestCommandName.length)}  ${command.description}`;
                })
                    .join('\n')
            });
            sections.push({
                title: `For more info, run any command with the \`--help\` flag`,
                body: commands
                    .map(command => `  $ ${name}${command.name === '' ? '' : ` ${command.name}`} --help`)
                    .join('\n')
            });
        }
        const options = this.isGlobalCommand
            ? globalOptions
            : [...this.options, ...(globalOptions || [])];
        if (options.length > 0) {
            const longestOptionName = findLongest(options.map(option => option.rawName));
            sections.push({
                title: 'Options',
                body: options
                    .map(option => {
                    return `  ${padRight(option.rawName, longestOptionName.length)}  ${option.description} ${option.config.default === undefined
                        ? ''
                        : `(default: ${option.config.default})`}`;
                })
                    .join('\n')
            });
        }
        if (this.examples.length > 0) {
            sections.push({
                title: 'Examples',
                body: this.examples
                    .map(example => {
                    if (typeof example === 'function') {
                        return example(name);
                    }
                    return example;
                })
                    .join('\n')
            });
        }
        if (helpCallback) {
            helpCallback(sections);
        }
        console.log(sections
            .map(section => {
            return section.title
                ? `${section.title}:\n${section.body}`
                : section.body;
        })
            .join('\n\n'));
        exit(0);
    }
    outputVersion() {
        const { name } = this.cli;
        const { versionNumber } = this.cli.globalCommand;
        if (versionNumber) {
            console.log(`${name}/${versionNumber} ${platformInfo}`);
        }
        exit(0);
    }
    checkRequiredArgs() {
        const minimalArgsCount = this.args.filter(arg => arg.required).length;
        if (this.cli.args.length < minimalArgsCount) {
            console.error(`error: missing required args for command \`${this.rawName}\``);
            exit(1);
        }
    }
    /**
     * Check if the parsed options contain any unknown options
     *
     * Exit and output error when true
     */
    checkUnknownOptions() {
        const { rawOptions, globalCommand } = this.cli;
        if (!this.config.allowUnknownOptions) {
            for (const name of Object.keys(rawOptions)) {
                if (name !== '--' &&
                    !this.hasOption(name) &&
                    !globalCommand.hasOption(name)) {
                    console.error(`error: Unknown option \`${name.length > 1 ? `--${name}` : `-${name}`}\``);
                    exit(1);
                }
            }
        }
    }
    /**
     * Check if the required string-type options exist
     */
    checkOptionValue() {
        const { rawOptions, globalCommand } = this.cli;
        const options = [...globalCommand.options, ...this.options];
        for (const option of options) {
            const value = rawOptions[option.name.split('.')[0]];
            // Check required option value
            if (option.required) {
                const hasNegated = options.some(o => o.negated && o.names.includes(option.name));
                if (value === true || (value === false && !hasNegated)) {
                    console.error(`error: option \`${option.rawName}\` value is missing`);
                    exit(1);
                }
            }
        }
    }
}
class GlobalCommand extends Command {
    constructor(cli) {
        super('@@global@@', '', {}, cli);
    }
}

class CAC extends EventEmitter {
    /**
     * @param name The program name to display in help and version message
     */
    constructor(name = '') {
        super();
        this.name = name;
        this.commands = [];
        this.globalCommand = new GlobalCommand(this);
        this.globalCommand.usage('<command> [options]');
    }
    /**
     * Add a global usage text.
     *
     * This is not used by sub-commands.
     */
    usage(text) {
        this.globalCommand.usage(text);
        return this;
    }
    /**
     * Add a sub-command
     */
    command(rawName, description, config) {
        const command = new Command(rawName, description || '', config, this);
        command.globalCommand = this.globalCommand;
        this.commands.push(command);
        return command;
    }
    /**
     * Add a global CLI option.
     *
     * Which is also applied to sub-commands.
     */
    option(rawName, description, config) {
        this.globalCommand.option(rawName, description, config);
        return this;
    }
    /**
     * Show help message when `-h, --help` flags appear.
     *
     */
    help(callback) {
        this.globalCommand.option('-h, --help', 'Display this message');
        this.globalCommand.helpCallback = callback;
        this.showHelpOnExit = true;
        return this;
    }
    /**
     * Show version number when `-v, --version` flags appear.
     *
     */
    version(version, customFlags = '-v, --version') {
        this.globalCommand.version(version, customFlags);
        this.showVersionOnExit = true;
        return this;
    }
    /**
     * Add a global example.
     *
     * This example added here will not be used by sub-commands.
     */
    example(example) {
        this.globalCommand.example(example);
        return this;
    }
    /**
     * Output the corresponding help message
     * When a sub-command is matched, output the help message for the command
     * Otherwise output the global one.
     *
     * This will also call `process.exit(0)` to quit the process.
     */
    outputHelp() {
        if (this.matchedCommand) {
            this.matchedCommand.outputHelp();
        }
        else {
            this.globalCommand.outputHelp();
        }
    }
    /**
     * Output the version number.
     *
     * This will also call `process.exit(0)` to quit the process.
     */
    outputVersion() {
        this.globalCommand.outputVersion();
    }
    setParsedInfo({ args, options, rawOptions }, matchedCommand, matchedCommandName) {
        this.args = args;
        this.options = options;
        this.rawOptions = rawOptions;
        if (matchedCommand) {
            this.matchedCommand = matchedCommand;
        }
        if (matchedCommandName) {
            this.matchedCommandName = matchedCommandName;
        }
        return this;
    }
    /**
     * Parse argv
     */
    parse(argv = processArgs, { 
    /** Whether to run the action for matched command */
    run = true } = {}) {
        this.rawArgs = argv;
        if (!this.name) {
            this.name = argv[1] ? getFileName(argv[1]) : 'cli';
        }
        let shouldParse = true;
        // Search sub-commands
        for (const command of this.commands) {
            const mriResult = this.mri(argv.slice(2), command);
            const commandName = mriResult.args[0];
            if (command.isMatched(commandName)) {
                shouldParse = false;
                const parsedInfo = Object.assign({}, mriResult, { args: mriResult.args.slice(1) });
                this.setParsedInfo(parsedInfo, command, commandName);
                this.emit(`command:${commandName}`, command);
            }
        }
        if (shouldParse) {
            // Search the default command
            for (const command of this.commands) {
                if (command.name === '') {
                    shouldParse = false;
                    const mriResult = this.mri(argv.slice(2), command);
                    this.setParsedInfo(mriResult, command);
                    this.emit(`command:!`, command);
                }
            }
        }
        if (shouldParse) {
            const mriResult = this.mri(argv.slice(2));
            this.setParsedInfo(mriResult);
        }
        if (this.options.help && this.showHelpOnExit) {
            this.outputHelp();
        }
        if (this.options.version && this.showVersionOnExit) {
            this.outputVersion();
        }
        const parsedArgv = { args: this.args, options: this.options };
        if (run) {
            this.runMatchedCommand();
        }
        if (!this.matchedCommand && this.args[0]) {
            this.emit('command:*');
        }
        return parsedArgv;
    }
    mri(argv, 
    /** Matched command */ command) {
        // All added options
        const cliOptions = [
            ...this.globalCommand.options,
            ...(command ? command.options : [])
        ];
        const mriOptions = getMriOptions(cliOptions);
        // Extract everything after `--` since mri doesn't support it
        let argsAfterDoubleDashes = [];
        const doubleDashesIndex = argv.indexOf('--');
        if (doubleDashesIndex > -1) {
            argsAfterDoubleDashes = argv.slice(doubleDashesIndex + 1);
            argv = argv.slice(0, doubleDashesIndex);
        }
        const parsed = lib(argv, mriOptions);
        const args = parsed._;
        delete parsed._;
        const options = {
            '--': argsAfterDoubleDashes
        };
        // Set option default value
        const ignoreDefault = command && command.config.ignoreOptionDefaultValue
            ? command.config.ignoreOptionDefaultValue
            : this.globalCommand.config.ignoreOptionDefaultValue;
        let transforms = Object.create(null);
        for (const cliOption of cliOptions) {
            if (!ignoreDefault && cliOption.config.default !== undefined) {
                for (const name of cliOption.names) {
                    options[name] = cliOption.config.default;
                }
            }
            // If options type is defined
            if (Array.isArray(cliOption.config.type)) {
                if (transforms[cliOption.name] === undefined) {
                    transforms[cliOption.name] = Object.create(null);
                    transforms[cliOption.name]['shouldTransform'] = true;
                    transforms[cliOption.name]['transformFunction'] =
                        cliOption.config.type[0];
                }
            }
        }
        // Camelcase option names and set dot nested option values
        for (const key of Object.keys(parsed)) {
            const keys = key.split('.').map((v, i) => {
                return i === 0 ? camelcase(v) : v;
            });
            setDotProp(options, keys, parsed[key]);
            setByType(options, transforms);
        }
        return {
            args,
            options,
            rawOptions: parsed
        };
    }
    runMatchedCommand() {
        const { args, options, matchedCommand: command } = this;
        if (!command || !command.commandAction)
            return;
        command.checkUnknownOptions();
        command.checkOptionValue();
        command.checkRequiredArgs();
        const actionArgs = [];
        command.args.forEach((arg, index) => {
            if (arg.variadic) {
                actionArgs.push(args.slice(index));
            }
            else {
                actionArgs.push(args[index]);
            }
        });
        actionArgs.push(options);
        return command.commandAction.apply(this, actionArgs);
    }
}

/**
 * @param name The program name to display in help and version message
 */
const cac = (name = '') => new CAC(name);
if (typeof module !== 'undefined') {
    module.exports = cac;
    module.exports.default = cac;
    module.exports.cac = cac;
}

export default cac;
export { cac };
