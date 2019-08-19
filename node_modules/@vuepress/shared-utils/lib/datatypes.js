"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const chalk_1 = __importDefault(require("chalk"));
exports.isObject = (obj) => obj !== null && typeof obj === 'object';
/**
 * Get the raw type string of a value e.g. [object Object]
 */
const _toString = Object.prototype.toString;
const getObjectType = (x) => _toString.call(x).slice(8, -1);
const isOfType = (type) => (x) => typeof x === type; // eslint-disable-line valid-typeof
const isObjectOfType = (type) => (x) => getObjectType(x) === type;
exports.isFunction = isOfType('function');
exports.isString = isOfType('string');
exports.isBoolean = isOfType('boolean');
exports.isPlainObject = isObjectOfType('Object');
exports.isUndefined = isOfType('undefined');
exports.isNull = (x) => x === null;
exports.isNullOrUndefined = (x) => exports.isUndefined(x) || exports.isNull(x);
exports.toRawType = (value) => _toString.call(value).slice(8, -1);
exports.getType = function (fn) {
    const match = fn && fn.toString().match(/^\s*function (\w+)/);
    return match ? match[1] : '';
};
function toNaturalMultiTypesLanguage(types) {
    const len = types.length;
    if (len === 1) {
        return types.join('');
    }
    const rest = types.slice(0, len - 1);
    const last = types[len - 1];
    return rest.join(', ') + ' or ' + last;
}
function assertTypes(value, types) {
    let valid;
    let warnMsg;
    let actualType = exports.toRawType(value);
    const expectedTypes = [];
    if (actualType === 'AsyncFunction') {
        actualType = 'Function';
    }
    for (const type of types) {
        const expectedType = exports.getType(type);
        expectedTypes.push(expectedType);
        valid = actualType === expectedType;
        if (valid)
            break;
    }
    if (!valid) {
        warnMsg =
            `expected a ${chalk_1.default.green(toNaturalMultiTypesLanguage(expectedTypes))} ` +
                `but got ${chalk_1.default.yellow(actualType)}.`;
    }
    return { valid, warnMsg };
}
exports.assertTypes = assertTypes;
