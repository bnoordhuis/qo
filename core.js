(function(globalThis) {
	const {request} = globalThis

	globalThis.get = function(...args) {
		return request("GET", ...args)
	}
})(globalThis)
