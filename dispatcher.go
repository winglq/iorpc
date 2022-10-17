package iorpc

import "errors"

var ErrUnknownService = errors.New("unknown service")

type Service uint32

type Dispatcher struct {
	services       []HandlerFunc
	serviceNameMap map[string]Service
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		services:       make([]HandlerFunc, 0),
		serviceNameMap: make(map[string]Service),
	}
}

func (d *Dispatcher) AddService(name string, handler HandlerFunc) (srv Service, conflict bool) {
	if _, conflict = d.serviceNameMap[name]; conflict {
		return 0, conflict
	}

	srv = Service(len(d.services))
	d.serviceNameMap[name] = srv
	d.services = append(d.services, handler)
	return srv, false
}

func (d *Dispatcher) GetService(name string) (Service, bool) {
	svc, ok := d.serviceNameMap[name]
	return svc, ok
}

func (d *Dispatcher) MustGetService(name string) Service {
	svc, ok := d.GetService(name)
	if !ok {
		panic("service not found: " + name)
	}
	return svc
}

func (d *Dispatcher) HandlerFunc() HandlerFunc {
	return func(clientAddr string, request Request) (response *Response, err error) {
		service := request.Service
		if int(service) >= len(d.services) {
			return nil, ErrUnknownService
		}
		return d.services[service](clientAddr, request)
	}
}
