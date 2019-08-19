'use strict';

module.exports = url => {
	if (typeof url !== 'string') {
		throw new TypeError(`Expected a \`string\`, got \`${typeof url}\``);
	}

	return /^[a-z][a-z\d+.-]*:/.test(url);
};
